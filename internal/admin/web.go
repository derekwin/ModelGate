package admin

import (
	"net/http"
)

func StartWebAdmin(router *http.ServeMux) {
	router.HandleFunc("/admin/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "admin/index.html")
	})
	router.HandleFunc("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
}
