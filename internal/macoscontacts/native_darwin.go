//go:build darwin && cgo

package macoscontacts

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=10.14
#cgo LDFLAGS: -framework Foundation -framework Contacts -framework AppKit
#include <stdlib.h>
int plContactsStatus(void);
char *plContactsRequest(void);
char *plContactsSearch(const char *query, int limit);
char *plContactsOpen(const char *identifier);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

type platformBackend struct{}

func (platformBackend) Status() string {
	switch int(C.plContactsStatus()) {
	case 0:
		return "not-determined"
	case 1:
		return "restricted"
	case 2:
		return "denied"
	case 3:
		return "authorized"
	case 4:
		return "limited"
	default:
		return "unavailable"
	}
}
func nativeError(value *C.char) error {
	if value == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(value))
	return errors.New(C.GoString(value))
}
func (platformBackend) Request() error { return nativeError(C.plContactsRequest()) }
func (platformBackend) Search(query string, limit int) (Result, error) {
	q := C.CString(query)
	defer C.free(unsafe.Pointer(q))
	raw := C.plContactsSearch(q, C.int(limit))
	defer C.free(unsafe.Pointer(raw))
	if raw == nil {
		return Result{}, errors.New("Contacts returned no response")
	}
	var response struct {
		Result
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(C.GoString(raw)), &response); err != nil {
		return Result{}, err
	}
	if response.Error != "" {
		return Result{}, errors.New(response.Error)
	}
	return response.Result, nil
}
func (platformBackend) Open(id string) error {
	value := C.CString(id)
	defer C.free(unsafe.Pointer(value))
	return nativeError(C.plContactsOpen(value))
}
