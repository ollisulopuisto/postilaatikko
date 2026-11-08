package v1

import "net/http"

// VirtualFolderUnreadCountOperations provides service-owned saved-view unread counts.
type VirtualFolderUnreadCountOperations interface {
	GetVirtualFolderUnreadCounts() (map[string]int, error)
}

// MountVirtualFolderUnreadCountRoute mounts the authenticated sidebar-count read.
func MountVirtualFolderUnreadCountRoute(mux *http.ServeMux, ops VirtualFolderUnreadCountOperations, requireCap func(http.ResponseWriter, *http.Request, Capability) bool) {
	mux.HandleFunc("GET /v1/views/unread-counts", func(w http.ResponseWriter, r *http.Request) {
		if !requireCap(w, r, CapabilityMailRead) {
			return
		}
		counts, err := ops.GetVirtualFolderUnreadCounts()
		if err != nil {
			writeHeaderError(w, r, http.StatusInternalServerError, CodeInternal, "Could not load virtual-folder unread counts", true)
			return
		}
		if counts == nil {
			counts = map[string]int{}
		}
		writeAPIResponse(w, r, http.StatusOK, counts)
	})
}
