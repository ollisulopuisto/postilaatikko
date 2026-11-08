package v1

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hkdb/aerion/internal/virtualfolder"
)

// VirtualFolderMutationRequest describes one persisted view operation.
type VirtualFolderMutationRequest struct {
	Operation     string                       `json:"operation"`
	VirtualFolder *virtualfolder.VirtualFolder `json:"virtualFolder,omitempty"`
	ID            string                       `json:"id,omitempty"`
	IDs           []string                     `json:"ids,omitempty"`
}

// VirtualFolderMutationOperations owns saved-view persistence and cache refresh.
type VirtualFolderMutationOperations interface {
	CreateVirtualFolder(*virtualfolder.VirtualFolder) error
	UpdateVirtualFolder(*virtualfolder.VirtualFolder) error
	DeleteVirtualFolder(string) error
	ReorderVirtualFolders([]string) error
	RefreshVirtualFolderCache(string) error
}

// MountVirtualFolderMutationRoutes mounts authenticated persisted-view operations.
func MountVirtualFolderMutationRoutes(mux *http.ServeMux, ops VirtualFolderMutationOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("/v1/views/mutations", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailWrite) {
			return
		}
		if r.Method != http.MethodPost {
			writeVirtualFolderError(w, r, http.StatusMethodNotAllowed, CodeInvalidRequest, "Method not allowed")
			return
		}
		var req VirtualFolderMutationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeVirtualFolderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Malformed virtual view request")
			return
		}
		var err error
		switch strings.TrimSpace(req.Operation) {
		case "create":
			if req.VirtualFolder == nil {
				err = fmt.Errorf("virtual folder is required")
			} else {
				err = ops.CreateVirtualFolder(req.VirtualFolder)
			}
		case "update":
			if req.VirtualFolder == nil {
				err = fmt.Errorf("virtual folder is required")
			} else {
				err = ops.UpdateVirtualFolder(req.VirtualFolder)
			}
		case "delete":
			err = ops.DeleteVirtualFolder(req.ID)
		case "reorder":
			err = ops.ReorderVirtualFolders(req.IDs)
		case "refresh":
			err = ops.RefreshVirtualFolderCache(req.ID)
		default:
			writeVirtualFolderError(w, r, http.StatusBadRequest, CodeInvalidRequest, "Unknown virtual view operation")
			return
		}
		if err != nil {
			writeVirtualFolderError(w, r, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
		writeVirtualFolderResponse(w, r, struct{}{})
	})
}

func writeVirtualFolderResponse(w http.ResponseWriter, r *http.Request, data any) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(Response[any]{Data: data, RequestID: reqID})
}

func writeVirtualFolderError(w http.ResponseWriter, r *http.Request, status int, code ErrorCode, message string) {
	reqID := r.Header.Get("X-Request-Id")
	if reqID == "" {
		reqID = "req-unknown"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: ErrorBody{Code: code, Message: message, RequestID: reqID}})
}
