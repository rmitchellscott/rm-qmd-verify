package qmldiff

/*
#cgo LDFLAGS: -L${SRCDIR}/../../lib -lqmldiff -ldl -lm
#include <stdlib.h>
#include <stdbool.h>

// C API function declarations for qmldiff
extern void qmldiff_set_version(const char* version);
extern void qmldiff_load_rules(const char* rules);
extern void qmldiff_set_external_loader(void (*external_loader)(const char *file_name));
extern bool qmldiff_add_external_diff(const char* change_file_contents, const char* file_identifier);
extern int qmldiff_build_change_files(const char* root_dir);
extern bool qmldiff_is_modified(const char* file_name);
extern void qmldiff_disable_slots_while_processing(void);
extern void qmldiff_enable_slots_while_processing(void);
extern char* qmldiff_process_file(const char* file_name, const char* raw_contents, size_t contents_size);
extern void qmldiff_start_saving_thread(void);

// Error collection functions
extern void qmldiff_enable_error_collection(void);
extern void qmldiff_disable_error_collection(void);
extern bool qmldiff_has_collection_errors(void);
extern void qmldiff_print_and_clear_collection_errors(void);
extern int qmldiff_get_error_count(void);
extern unsigned long long qmldiff_get_error_hash(int index);
extern char* qmldiff_get_error_file(int index);
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// SetVersion sets the system version for version-specific diff filtering
func SetVersion(version string) {
	cVersion := C.CString(version)
	defer C.free(unsafe.Pointer(cVersion))
	C.qmldiff_set_version(cVersion)
}

// LoadRules configures hashtab rules (passed as raw string, not file path)
func LoadRules(rules string) {
	cRules := C.CString(rules)
	defer C.free(unsafe.Pointer(cRules))
	C.qmldiff_load_rules(cRules)
}

// AddExternalDiff adds a diff from external source (not from filesystem)
func AddExternalDiff(changeFileContents, fileIdentifier string) error {
	cContents := C.CString(changeFileContents)
	defer C.free(unsafe.Pointer(cContents))
	cIdentifier := C.CString(fileIdentifier)
	defer C.free(unsafe.Pointer(cIdentifier))

	success := C.qmldiff_add_external_diff(cContents, cIdentifier)
	if !success {
		return fmt.Errorf("failed to add external diff: %s", fileIdentifier)
	}
	return nil
}

// BuildChangeFiles loads all .qmd diff files from rootDir
// Also loads hashtab from rootDir/hashtab if present
// Returns the number of diff files successfully loaded
func BuildChangeFiles(rootDir string) (int, error) {
	cRootDir := C.CString(rootDir)
	defer C.free(unsafe.Pointer(cRootDir))

	count := C.qmldiff_build_change_files(cRootDir)
	if count == 0 {
		return 0, errors.New("failed to load any change files")
	}
	return int(count), nil
}

// IsModified checks if any loaded diff affects the given file
// Returns true if file will be modified, false otherwise
func IsModified(fileName string) bool {
	cFileName := C.CString(fileName)
	defer C.free(unsafe.Pointer(cFileName))

	return bool(C.qmldiff_is_modified(cFileName))
}

// DisableSlotsWhileProcessing disables slot processing temporarily
func DisableSlotsWhileProcessing() {
	C.qmldiff_disable_slots_while_processing()
}

// EnableSlotsWhileProcessing re-enables slot processing after being disabled
func EnableSlotsWhileProcessing() {
	C.qmldiff_enable_slots_while_processing()
}

// ProcessFileResult contains the result of processing a file
type ProcessFileResult struct {
	// Modified indicates whether the file was changed
	Modified bool
	// Content contains the modified QML content (only if Modified is true)
	Content string
	// Error contains any error that occurred during processing
	Error error
}

// ProcessFile processes a QML file and applies all relevant diffs
// Returns ProcessFileResult with the modified content or an error
func ProcessFile(fileName, rawContents string) ProcessFileResult {
	cFileName := C.CString(fileName)
	defer C.free(unsafe.Pointer(cFileName))
	cContents := C.CString(rawContents)
	defer C.free(unsafe.Pointer(cContents))

	result := C.qmldiff_process_file(cFileName, cContents, C.size_t(len(rawContents)))

	if result == nil {
		// Either no changes or error occurred
		// Check if file was supposed to be modified
		if IsModified(fileName) {
			return ProcessFileResult{
				Modified: false,
				Error:    fmt.Errorf("processing failed for %s", fileName),
			}
		}
		return ProcessFileResult{
			Modified: false,
		}
	}

	// Convert C string to Go string and free the C string
	content := C.GoString(result)
	C.free(unsafe.Pointer(result))

	return ProcessFileResult{
		Modified: true,
		Content:  content,
	}
}

// StartSavingThread starts background thread for hashtab export
// Only used when QMLDIFF_HASHTAB_CREATE env var is set
func StartSavingThread() {
	C.qmldiff_start_saving_thread()
}

// EnableErrorCollection enables collection of hash lookup errors
func EnableErrorCollection() {
	C.qmldiff_enable_error_collection()
}

// DisableErrorCollection disables collection of hash lookup errors
func DisableErrorCollection() {
	C.qmldiff_disable_error_collection()
}

// HasCollectionErrors returns true if any hash lookup errors were collected
func HasCollectionErrors() bool {
	return bool(C.qmldiff_has_collection_errors())
}

// PrintAndClearCollectionErrors prints collected errors to stderr and clears them
func PrintAndClearCollectionErrors() {
	C.qmldiff_print_and_clear_collection_errors()
}

// GetErrorCount returns the number of collected errors
func GetErrorCount() int {
	return int(C.qmldiff_get_error_count())
}

// GetErrorHash returns the hash ID for the error at the given index
// Returns 0 if index is out of bounds
func GetErrorHash(index int) uint64 {
	return uint64(C.qmldiff_get_error_hash(C.int(index)))
}

// GetErrorFile returns the source file path for the error at the given index
// Returns empty string if index is out of bounds
// Note: The returned string is cached in Rust and does not need to be freed
func GetErrorFile(index int) string {
	cFile := C.qmldiff_get_error_file(C.int(index))
	if cFile == nil {
		return ""
	}
	return C.GoString(cFile)
}
