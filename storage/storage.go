// storage/storage.go
package storage

import (
	"bytes" // Added for createFolder body
	"encoding/base64"
	"encoding/json" // Added for API interaction
	"errors"        // Added for ErrNotFound
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync" // Added for RWMutex

	"github.com/schollz/progressbar/v3"
)

var ErrNotFound = errors.New("item not found")

// StorageBackend defines an interface for saving files
type StorageBackend interface {
	EnsureDirectory(path string) error
	UploadFile(filePath, remotePath, locationId, note string) error
	SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error)
}

type RemoteStorageBackend struct {
	AuthToken      string
	directoryCache map[string]map[string]string // parentId -> folderName -> folderId
	cacheMutex     sync.RWMutex                 // Mutex for thread-safe access to the cache
}

// InitializeCache initializes the directory cache by fetching the structure starting from the base folder ID.
func (rsb *RemoteStorageBackend) InitializeCache(baseFolderId string) error {
	// Initialize the cache map
	rsb.directoryCache = make(map[string]map[string]string)

	// Start recursive fetch and cache
	if err := rsb.fetchAndCacheDirectoryRecursive(baseFolderId); err != nil {
		return fmt.Errorf("InitializeCache: failed to fetch and cache directory structure starting from base folder ID '%s': %w", baseFolderId, err)
	}

	return nil
}

// fetchAndCacheDirectoryRecursive recursively fetches and caches the directory structure starting from the given parentId.
func (rsb *RemoteStorageBackend) fetchAndCacheDirectoryRecursive(parentId string) error {
	baseURL := "https://fuckingfast.net/api/fs"
	var fullURL string

	// Handle root case
	if parentId == "" || parentId == "fs" {
		fullURL = baseURL
	} else {
		fullURL = fmt.Sprintf("%s/%s", baseURL, url.PathEscape(parentId))
	}

	// Create GET request
	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return fmt.Errorf("fetchAndCacheDirectoryRecursive: failed to create GET request for parent %s: %w", parentId, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rsb.AuthToken))

	// Execute request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetchAndCacheDirectoryRecursive: failed to execute GET request for parent %s: %w", parentId, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("fetchAndCacheDirectoryRecursive: unexpected status code %d for parent %s: %s", resp.StatusCode, parentId, string(bodyBytes))
	}

	// Define response structure
	type Child struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		IsDirectory bool   `json:"isDirectory"`
	}
	type DataResponse struct {
		Data struct {
			Children []Child `json:"children"`
		} `json:"data"`
	}

	// Read and parse the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	fmt.Printf("DEBUG: Raw response for parent %s: %s\n", parentId, string(bodyBytes)) // Keep debug for now
	if err != nil {
		return fmt.Errorf("fetchAndCacheDirectoryRecursive: failed to read response body for parent %s: %w", parentId, err)
	}

	var respData DataResponse
	if err := json.Unmarshal(bodyBytes, &respData); err != nil {
		return fmt.Errorf("fetchAndCacheDirectoryRecursive: failed to parse response for parent %s: %w", parentId, err)
	}

	itemsToSearch := respData.Data.Children

	// Cache directories
	rsb.cacheMutex.Lock()
	if rsb.directoryCache[parentId] == nil {
		rsb.directoryCache[parentId] = make(map[string]string)
	}
	for _, item := range itemsToSearch {
		if item.IsDirectory {
			rsb.directoryCache[parentId][item.Name] = item.ID
		}
	}
	rsb.cacheMutex.Unlock()

	// Recurse into subdirectories
	for _, item := range itemsToSearch {
		if item.IsDirectory {
			if err := rsb.fetchAndCacheDirectoryRecursive(item.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- Folder Management Methods ---

// findFolderIdByName retrieves the folder ID by name within a parent folder.
// It fetches the parent directory's contents and searches for the name.
func (rsb *RemoteStorageBackend) findFolderIdByName(parentId, name string) (string, error) {
	if parentId == "" {
		// If parentId is empty, we might be looking in the root.
		// The API endpoint for root is /api/fs, not /api/fs/
		parentId = "fs" // Use "fs" as a placeholder to construct the correct root URL
	}
	baseURL := "https://fuckingfast.net/api/fs"
	// Construct URL carefully: /api/fs for root, /api/fs/{parentId} otherwise
	var fullURL string
	if parentId == "fs" {
		fullURL = baseURL
	} else {
		fullURL = fmt.Sprintf("%s/%s", baseURL, url.PathEscape(parentId))
	}

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return "", fmt.Errorf("findFolderIdByName: failed to create GET request for parent %s: %w", parentId, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rsb.AuthToken))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("findFolderIdByName: failed to execute GET request for parent %s: %w", parentId, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrNotFound // Parent directory itself not found
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("findFolderIdByName: unexpected status code %d when getting parent %s: %s", resp.StatusCode, parentId, string(bodyBytes))
	}

	// Define potential response structures
	type ItemInfo struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type string `json:"type"`
	}
	// Nested under "data": {"files": [...]}
	type FindResponseNestedObject struct {
		Data struct {
			Files []ItemInfo `json:"files"`
		} `json:"data"`
	}
	// Nested under "data": [...]
	type FindResponseNestedArray struct {
		Data []ItemInfo `json:"data"`
	}
	// Top-level: {"files": [...]}
	type FindResponseTopLevelObject struct {
		Files []ItemInfo `json:"files"`
	}
	// Top-level: [...]
	type FindResponseTopLevelArray []ItemInfo

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("findFolderIdByName: failed to read response body for parent %s: %w", parentId, err)
	}
	// fmt.Printf("DEBUG: Response body for parent %s: %s\n", parentId, string(bodyBytes)) // Debugging line
	var itemsToSearch []ItemInfo
	structureMatched := false // Renamed for clarity

	// Try decoding nested object structure first: {"data": {"files": [...]}}
	var nestedObjectResp FindResponseNestedObject
	if errNestedObj := json.Unmarshal(bodyBytes, &nestedObjectResp); errNestedObj == nil {
		// Check if Data and Files are not nil before accessing
		if nestedObjectResp.Data.Files != nil {
			itemsToSearch = nestedObjectResp.Data.Files
		}
		structureMatched = true
		// fmt.Printf("DEBUG: Matched nested object structure. Items: %d\n", len(itemsToSearch)) // Debug
	}

	// Try decoding nested array structure: {"data": [...]}
	if !structureMatched {
		var nestedArrayResp FindResponseNestedArray
		if errNestedArr := json.Unmarshal(bodyBytes, &nestedArrayResp); errNestedArr == nil {
			if nestedArrayResp.Data != nil {
				itemsToSearch = nestedArrayResp.Data
			}
			structureMatched = true
			// fmt.Printf("DEBUG: Matched nested array structure. Items: %d\n", len(itemsToSearch)) // Debug
		}
	}

	// Try decoding top-level object structure: {"files": [...]}
	if !structureMatched {
		var topLevelObjectResp FindResponseTopLevelObject
		if errTopLevelObj := json.Unmarshal(bodyBytes, &topLevelObjectResp); errTopLevelObj == nil {
			if topLevelObjectResp.Files != nil {
				itemsToSearch = topLevelObjectResp.Files
			}
			structureMatched = true
			// fmt.Printf("DEBUG: Matched top-level object structure. Items: %d\n", len(itemsToSearch)) // Debug
		}
	}

	// Try decoding top-level array structure: [...]
	if !structureMatched {
		var topLevelArrayResp FindResponseTopLevelArray
		if errTopLevelArr := json.Unmarshal(bodyBytes, &topLevelArrayResp); errTopLevelArr == nil {
			// No nil check needed for top-level array itself
			itemsToSearch = topLevelArrayResp
			structureMatched = true
			// fmt.Printf("DEBUG: Matched top-level array structure. Items: %d\n", len(itemsToSearch)) // Debug
		}
	}

	// If none of the structures could be successfully unmarshalled
	if !structureMatched {
		// This indicates a JSON parsing error or a completely unknown structure
		// It's different from finding an empty directory listing.
		// fmt.Printf("DEBUG: findFolderIdByName: Failed to decode response for parent %s into any known structure. Body: %s\n", parentId, string(bodyBytes)) // Debugging
		// Return specific error? Or treat as not found? Treating as not found for now.
		return "", ErrNotFound // Treat inability to parse known structures as "not found"
	}

	// If a structure was matched, but itemsToSearch is still nil (e.g., {"data": {"files": null}}), initialize it
	if itemsToSearch == nil {
		itemsToSearch = []ItemInfo{} // Ensure it's an empty slice, not nil
	}

	// Iterate through the extracted items (which might be an empty slice)
	for _, item := range itemsToSearch {
		// Check if the item is a directory and the name matches
		// Assuming the type field is "directory" for folders. Adjust if needed.
		if item.Type == "directory" && item.Name == name {
			return item.ID, nil
		}
	}

	// If loop completes without finding the folder
	return "", ErrNotFound
}

// createFolder creates a new folder under the specified parent folder.
// It adjusts the endpoint and payload based on whether a parentId is provided.
func (rsb *RemoteStorageBackend) createFolder(parentId, name string) (string, error) {
	baseURL := "https://fuckingfast.net/api/fs"
	var targetURL string
	var payload interface{}

	if parentId == "" {
		// Creating in root/base: Use POST /api/fs
		targetURL = baseURL
		// Send only name, assuming API defaults parent for root creation
		payload = struct {
			Name string `json:"name"`
		}{
			Name: name,
		}
		// log.Printf("createFolder: Attempting root creation for '%s' at %s", name, targetURL) // Optional logging
	} else {
		// Creating subdirectory: Try POST /api/fs/{parentId}
		targetURL = fmt.Sprintf("%s/%s", baseURL, url.PathEscape(parentId))
		// Send only name in payload, as parentId is in the URL
		payload = struct {
			Name string `json:"name"`
		}{
			Name: name,
		}
		// log.Printf("createFolder: Attempting subdirectory creation for '%s' under '%s' at %s", name, parentId, targetURL) // Optional logging
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("createFolder: failed to marshal payload for folder '%s' (parentId: '%s'): %w", name, parentId, err)
	}

	req, err := http.NewRequest("POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("createFolder: failed to create POST request for folder '%s' at %s: %w", name, targetURL, err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rsb.AuthToken))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("createFolder: failed to execute POST request for folder '%s' in parent %s: %w", name, parentId, err)
	}
	defer resp.Body.Close()

	// Define struct for expected success response, matching the nested structure {"data": {"id": ...}}
	var successResponse struct {
		Code int `json:"code"` // Optional: capture code if needed
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	respBodyBytes, err := io.ReadAll(resp.Body) // Read body for potential error messages or successful decoding
	if err != nil {
		return "", fmt.Errorf("createFolder: failed to read response body for folder '%s' in parent %s: %w", name, parentId, err)
	}

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		// Use the successResponse struct defined earlier
		if err := json.Unmarshal(respBodyBytes, &successResponse); err != nil {
			// Use targetURL in error message for consistency
			return "", fmt.Errorf("createFolder: failed to decode successful response for folder '%s' at %s: %w. Body: %s", name, targetURL, err, string(respBodyBytes))
		}
		// Check the nested ID
		if successResponse.Data.ID == "" {
			// Use targetURL in error message for consistency
			return "", fmt.Errorf("createFolder: successful response for folder '%s' at %s but data.id is empty. Body: %s", name, targetURL, string(respBodyBytes))
		}
		return successResponse.Data.ID, nil
	} else if resp.StatusCode == http.StatusConflict {
		// If conflict, try to find the existing folder
		existingId, findErr := rsb.findFolderIdByName(parentId, name)
		if findErr != nil {
			// If finding fails after a conflict, return a combined error
			return "", fmt.Errorf("createFolder: conflict creating folder '%s' in parent %s, but failed to find existing: %w. Original conflict body: %s", name, parentId, findErr, string(respBodyBytes))
		}
		if existingId == "" {
			// This case should ideally not happen if findFolderIdByName works correctly after a 409
			return "", fmt.Errorf("createFolder: conflict creating folder '%s' in parent %s, found existing but ID is empty. Original conflict body: %s", name, parentId, string(respBodyBytes))
		}
		return existingId, nil // Return the ID found after conflict
	}

	// Handle other unexpected errors
	return "", fmt.Errorf("createFolder: unexpected status code %d when creating folder '%s' in parent %s: %s", resp.StatusCode, name, parentId, string(respBodyBytes))
}

// --- End Folder Management Methods ---

// Updated UploadFile to include progress bar
func (rsb *RemoteStorageBackend) UploadFile(localFilePath, remotePath, locationId, note string) error {
	// Plan Step 6: Rename remotePath parameter to filename
	filename := remotePath // Keep original remotePath for logging/errors if needed
	if len(filename) > 1024 {
		return fmt.Errorf("remote path exceeds the maximum length of 1024 characters")
	}

	baseURL := "https://w.fuckingfast.net/" // Base URL without trailing slash

	// The complex path encoding logic (calculating encodedPathSegment) is removed as the filename is now just the last part.
	// The directory structure is handled by locationId.

	// Construct the final URL with the properly encoded path segment
	// Ensure there's exactly one slash between base and path
	// Plan Step 6: Construct URL using filename and locationId query param
	// The complex path encoding logic is removed as the filename is now just the last part.
	// The directory structure is handled by locationId.
	finalURLStr := strings.TrimSuffix(baseURL, "/") + "/" + url.PathEscape(filename)

	// --- Query Parameter Handling ---
	finalURL, err := url.Parse(finalURLStr)
	if err != nil {
		return fmt.Errorf("failed to parse constructed URL '%s': %w", finalURLStr, err)
	}
	queryParams := url.Values{}
	if locationId != "" {
		queryParams.Add("locationId", locationId)
	} else if note != "" {
		encodedNote := base64.StdEncoding.EncodeToString([]byte(note))
		queryParams.Add("note", encodedNote)
	}
	finalURL.RawQuery = queryParams.Encode() // Assign encoded query params

	// Open the local file
	file, err := os.Open(localFilePath)
	if err != nil {
		return fmt.Errorf("failed to open file '%s': %w", localFilePath, err)
	}
	defer file.Close()

	// Get file info for size and progress bar
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for '%s': %w", localFilePath, err)
	}
	fileSize := fileInfo.Size()

	// --- Progress Bar Setup ---
	uploadBar := progressbar.NewOptions(
		int(fileSize), // Use int() for compatibility, progressbar expects int
		progressbar.OptionSetDescription(fmt.Sprintf("[Uploading %s] ", filename)), // Use filename
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[blue]=[reset]", // Different color for upload?
			SaucerHead:    "[blue]>[reset]",
			SaucerPadding: " ",
			BarStart:      "|",
			BarEnd:        "|",
		}),
	)
	// Wrap the file reader with the progress bar reader
	progressReader := progressbar.NewReader(file, uploadBar)
	// --- End Progress Bar Setup ---

	// Use the final URL object's string representation
	req, err := http.NewRequest("PUT", finalURL.String(), &progressReader)
	if err != nil {
		fmt.Println() // Newline after potential bar init
		return fmt.Errorf("failed to create HTTP request for '%s': %w", finalURL.String(), err)
	}
	req.ContentLength = fileSize // Set ContentLength explicitly

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", rsb.AuthToken))
	// Optional Content-Type
	// mimeType := mime.TypeByExtension(path.Ext(remotePath))
	// if mimeType != "" {
	// 	req.Header.Set("Content-Type", mimeType)
	// }

	client := &http.Client{}
	fmt.Printf("DEBUG: Sending HTTP PUT request to %s\n", finalURL.String()) // Debug log before request
	resp, err := client.Do(req)
	fmt.Println()                                                                          // Ensure newline after progress bar finishes or errors out
	fmt.Printf("DEBUG: Received response for HTTP PUT request to %s\n", finalURL.String()) // Debug log after response
	if err != nil {
		return fmt.Errorf("failed to execute HTTP request for '%s': %w", finalURL.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload of '%s' failed with status: %s, body: %s", filename, resp.Status, string(bodyBytes))
	}

	return nil
}

// EnsureDirectory is deprecated for RemoteStorageBackend with folder IDs.
// Use ensureRemoteDirectoryRecursive instead.
func (rsb *RemoteStorageBackend) EnsureDirectory(path string) error {
	// This method is no longer suitable for the folder ID approach.
	// Consider logging a warning or returning an error if called.
	// log.Printf("Warning: EnsureDirectory called on RemoteStorageBackend, which now uses folder IDs. This method is deprecated.")
	return fmt.Errorf("EnsureDirectory is deprecated for RemoteStorageBackend with folder IDs; use ensureRemoteDirectoryRecursive")
}

// ensureRemoteDirectoryRecursive ensures the directory structure exists using folder IDs
// and returns the ID of the final directory in the path.
func (rsb *RemoteStorageBackend) ensureRemoteDirectoryRecursive(baseFolderId, relativePath string) (string, error) {
	// Clean the path and split into segments
	rsb.cacheMutex.RLock() // Acquire read lock for cache access
	defer rsb.cacheMutex.RUnlock()

	cleanRelativePath := path.Clean(relativePath)
	// Handle edge cases like ".", "/", ""
	if cleanRelativePath == "." || cleanRelativePath == "/" || cleanRelativePath == "" {
		return baseFolderId, nil // No subdirectories needed
	}
	segments := strings.Split(cleanRelativePath, "/")

	currentParentId := baseFolderId
	// Removed unused 'var err error' declaration

	for _, segment := range segments {
		// Skip empty segments that might result from splitting "//" or leading/trailing "/"
		if segment == "" || segment == "." {
			continue
		}

		// Check cache for the folder
		foundId := ""
		if rsb.directoryCache[currentParentId] != nil {
			foundId = rsb.directoryCache[currentParentId][segment]
		}

		if foundId == "" {
			// Folder doesn't exist in cache, create it
			newId, createErr := rsb.createFolder(currentParentId, segment)
			if createErr != nil {
				// If creation fails
				return "", fmt.Errorf("ensureRemoteDirectoryRecursive: error creating folder '%s' in parent %s: %w", segment, currentParentId, createErr)
			}
			if newId == "" {
				// If creation succeeded but returned an empty ID (should not happen with current createFolder logic)
				return "", fmt.Errorf("ensureRemoteDirectoryRecursive: created folder '%s' in parent %s but received empty ID", segment, currentParentId)
			}
			// Update cache with the new folder
			rsb.cacheMutex.Lock()
			if rsb.directoryCache[currentParentId] == nil {
				rsb.directoryCache[currentParentId] = make(map[string]string)
			}
			rsb.directoryCache[currentParentId][segment] = newId
			rsb.cacheMutex.Unlock()

			currentParentId = newId
		} else {
			// Folder was found
			if foundId == "" {
				// If found but ID is empty (should not happen with current findFolderIdByName logic)
				return "", fmt.Errorf("ensureRemoteDirectoryRecursive: found folder '%s' in parent %s but ID is empty", segment, currentParentId)
			}
			currentParentId = foundId // Update parent ID to the found folder's ID
		}
	}

	// After iterating through all segments, currentParentId holds the ID of the final directory
	return currentParentId, nil
}

// SaveFile implementation for RemoteStorageBackend using Folder IDs
func (rsb *RemoteStorageBackend) SaveFile(body io.Reader, baseFolderId, relativePath, fileName string) (string, error) {
	// Plan Step 5: Rename baseStoragePath to baseFolderId (already done in signature)

	// Plan Step 5: Ensure the target directory exists using folder IDs
	targetFolderId, err := rsb.ensureRemoteDirectoryRecursive(baseFolderId, relativePath)
	if err != nil {
		return "", fmt.Errorf("SaveFile: failed to ensure remote directory structure for path '%s' starting from base %s: %w", relativePath, baseFolderId, err)
	}
	if targetFolderId == "" {
		return "", fmt.Errorf("SaveFile: ensureRemoteDirectoryRecursive returned empty target folder ID for path '%s'", relativePath)
	}

	// Plan Step 5: Create temp file (existing logic)
	tempFile, err := os.CreateTemp("", "civitai-upload-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempFilePath := tempFile.Name()
	defer os.Remove(tempFilePath)

	_, err = io.Copy(tempFile, body)
	closeErr := tempFile.Close()
	if err != nil {
		return "", fmt.Errorf("failed to write to temporary file %s: %w", tempFilePath, err)
	}
	if closeErr != nil {
		return "", fmt.Errorf("failed to close temporary file %s: %w", tempFilePath, closeErr)
	}

	// Plan Step 5: Call UploadFile with correct arguments: temp path, filename, targetFolderId
	err = rsb.UploadFile(tempFilePath, fileName, targetFolderId, "") // Pass targetFolderId as locationId
	if err != nil {
		// Provide more context in the error message
		return "", fmt.Errorf("SaveFile: failed to upload file '%s' to folder ID '%s' from temp path '%s': %w", fileName, targetFolderId, tempFilePath, err)
	}

	// Plan Step 5: Return the logical remote path, not an ID
	logicalRemotePath := path.Join(relativePath, fileName)
	// Ensure forward slashes for consistency, even on Windows
	logicalRemotePath = filepath.ToSlash(logicalRemotePath)
	return logicalRemotePath, nil
}

// LocalStorageBackend implementation remains the same
type LocalStorageBackend struct{}

func (lsb *LocalStorageBackend) UploadFile(filePath, remotePath, locationId, note string) error {
	return fmt.Errorf("UploadFile is not supported for LocalStorageBackend")
}

func (lsb *LocalStorageBackend) EnsureDirectory(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create directory '%s': %w", path, err)
		}
	}
	return nil
}

func (lsb *LocalStorageBackend) SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error) {
	fullDirPath := filepath.Join(baseStoragePath, relativePath)

	if err := lsb.EnsureDirectory(fullDirPath); err != nil { // Call EnsureDirectory directly
		return "", fmt.Errorf("failed to ensure local directory '%s': %w", fullDirPath, err)
	}

	fullFilePath := filepath.Join(fullDirPath, fileName)

	file, err := os.Create(fullFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to create local file '%s': %w", fullFilePath, err)
	}
	defer file.Close()

	_, err = io.Copy(file, body)
	if err != nil {
		os.Remove(fullFilePath)
		return "", fmt.Errorf("failed to write to local file '%s': %w", fullFilePath, err)
	}

	return fullFilePath, nil
}

// PomfStorageBackend implements file storage using pomf.lain.la
type PomfStorageBackend struct{}

// PomfFile represents a single file entry in the Pomf API response
type PomfFile struct {
	Hash string `json:"hash"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// PomfResponse represents the overall JSON structure from the Pomf API
type PomfResponse struct {
	Success bool       `json:"success"`
	Files   []PomfFile `json:"files"`
}

// EnsureDirectory is a no-op for PomfStorageBackend
func (psb *PomfStorageBackend) EnsureDirectory(path string) error {
	return nil
}

// UploadFile uploads a file to pomf.lain.la using its file path
func (psb *PomfStorageBackend) UploadFile(filePath, remotePath, locationId, note string) error {
	// Implementation will be added later if needed
	return nil
}

// SaveFile uploads a file to pomf.lain.la using an io.Reader
func (psb *PomfStorageBackend) SaveFile(body io.Reader, baseStoragePath, relativePath, fileName string) (string, error) {
	// Note: baseStoragePath and relativePath are ignored as Pomf doesn't use filesystem paths
	// Create a temporary file
	tempFile, err := os.CreateTemp("", "pomf-upload-*.tmp")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	// Copy data to the temporary file
	if _, err := io.Copy(tempFile, body); err != nil {
		return "", fmt.Errorf("failed to write to temporary file: %w", err)
	}

	// Prepare multipart form data
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	formFile, err := writer.CreateFormFile("files[]", fileName)

	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	// Seek back to the start of the temp file to read its content
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("failed to seek temporary file: %w", err)
	}

	// Copy the temp file content into the multipart form field
	// This writes the file data into the requestBody buffer
	if _, err := io.Copy(formFile, tempFile); err != nil {
		return "", fmt.Errorf("failed to copy file content to form: %w", err)
	}

	// Close the multipart writer to finalize the request body
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// --- Progress Bar Setup ---
	// The requestBody buffer now contains the complete multipart payload
	bodySize := int64(requestBody.Len()) // Get the size of the complete payload

	uploadBar := progressbar.NewOptions(
		int(bodySize), // Use the actual payload size
		progressbar.OptionSetDescription(fmt.Sprintf("[Uploading %s] ", fileName)),
		progressbar.OptionSetWidth(15),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "[blue]=[reset]",
			SaucerHead:    "[blue]>[reset]",
			SaucerPadding: " ",
			BarStart:      "|",
			BarEnd:        "|",
		}),
	)

	// Wrap the requestBody buffer with the progress bar reader
	// The HTTP client will read from this, triggering progress updates
	progressReader := progressbar.NewReader(&requestBody, uploadBar)
	// --- End Progress Bar Setup ---

	// Create HTTP request with the progressReader as the body
	req, err := http.NewRequest("POST", "https://pomf.lain.la/upload.php", &progressReader) // Use progressReader here
	if err != nil {
		fmt.Println() // Newline if progress bar started but request creation failed
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.ContentLength = bodySize // Set the calculated ContentLength

	// Execute HTTP request
	resp, err := http.DefaultClient.Do(req)
	fmt.Println() // Ensure newline after progress bar finishes or errors out
	if err != nil {
		return "", fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Parse JSON response
	var pomfResponse PomfResponse
	if err := json.NewDecoder(resp.Body).Decode(&pomfResponse); err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	if !pomfResponse.Success || len(pomfResponse.Files) == 0 {
		return "", fmt.Errorf("upload failed or no files returned in response")
	}

	return pomfResponse.Files[0].URL, nil
}
