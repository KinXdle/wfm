package main

import (
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

func wfm(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	user, rw := auth(w, r)
	if user == "" {
		return
	}
	go log.Printf("req from=%q user=%q uri=%q form=%v", r.RemoteAddr, user, r.RequestURI, noText(r.Form))
	modern := false
	if strings.HasPrefix(r.UserAgent(), "Mozilla/5") {
		modern = true
	}

	uDir := filepath.Clean(r.FormValue("dir"))
	if uDir == "" || uDir == "." {
		uDir = "/"
	}
	eSort := url.QueryEscape(r.FormValue("sort"))
	uFp := filepath.Clean(r.FormValue("fp"))
	uBn := filepath.Base(r.FormValue("file"))
	hi := filepath.Base(r.FormValue("hi"))

	// button clicked
	switch {
	case r.FormValue("mkd") != "":
		prompt(w, uDir, "", eSort, "mkdir", nil)
		return
	case r.FormValue("mkf") != "":
		prompt(w, uDir, "", eSort, "mkfile", nil)
		return
	case r.FormValue("mkb") != "":
		prompt(w, uDir, "", eSort, "mkurl", nil)
		return
	case r.FormValue("mdelp") != "":
		prompt(w, uDir, "", eSort, "multi_delete", r.Form["mulf"])
		return
	case r.FormValue("mmovp") != "":
		prompt(w, uDir, "", eSort, "multi_move", r.Form["mulf"])
		return
	case r.FormValue("upload") != "":
		f, h, err := r.FormFile("filename")
		if err != nil {
			htErr(w, "upload", err)
			return
		}
		uploadFile(w, uDir, eSort, h, f, rw)
		return
	case r.FormValue("save") != "":
		saveText(w, uDir, eSort, uFp, r.FormValue("text"), rw)
		return
	case r.FormValue("home") != "":
		listFiles(w, "/", eSort, user, hi, modern)
		return
	case r.FormValue("up") != "":
		listFiles(w, filepath.Dir(uDir), eSort, hi, user, modern)
		return
	case r.FormValue("cancel") != "":
		listFiles(w, uDir, eSort, user, hi, modern)
		return
	}

	// form action
	switch r.FormValue("fn") {
	case "disp":
		dispFile(w, uFp)
	case "down":
		downFile(w, uFp)
	case "edit":
		editText(w, uFp, eSort)
	case "mkdir":
		mkdir(w, uDir, uBn, eSort, rw)
	case "mkfile":
		mkfile(w, uDir, uBn, eSort, rw)
	case "mkurl":
		mkurl(w, uDir, uBn, r.FormValue("url"), eSort, rw)
	case "rename":
		renFile(w, uDir, uBn, r.FormValue("dst"), eSort, rw)
	case "renp":
		prompt(w, uDir, r.FormValue("oldf"), eSort, "rename", nil)
	case "movp":
		prompt(w, uDir, uBn, eSort, "move", nil)
	case "delp":
		prompt(w, uDir, uBn, eSort, "delete", nil)
	case "move":
		log.Printf("move dir=%v file=%v user=%v@%v", uDir, uFp, user, r.RemoteAddr)
		moveFiles(w, uDir, []string{uBn}, r.FormValue("dst"), eSort, rw)
	case "delete":
		log.Printf("delete dir=%v file=%v user=%v@%v", uDir, uBn, user, r.RemoteAddr)
		deleteFiles(w, uDir, []string{uBn}, eSort, rw)
	case "multi_delete":
		log.Printf("multi_delete dir=%v files=%+v user=%v@%v", uDir, r.Form["mulf"], user, r.RemoteAddr)
		deleteFiles(w, uDir, r.Form["mulf"], eSort, rw)
	case "multi_move":
		log.Printf("multi_move dir=%v files=%+v dest=%v user=%v@%v", uDir, r.Form["mulf"], r.FormValue("dst"), user, r.RemoteAddr)
		moveFiles(w, uDir, r.Form["mulf"], r.FormValue("dst"), eSort, rw)
	case "logout":
		logout(w)
	case "about":
		about(w, uDir, eSort, r.UserAgent())
	default:
		listFiles(w, uDir, eSort, hi, user, modern)
	}
}

func favicon(w http.ResponseWriter, r *http.Request) {
	dispFavIcon(w)
}

func noText(m map[string][]string) map[string][]string {
	o := make(map[string][]string)
	for k, v := range m {
		if k == "text" {
			continue
		}
		o[k] = v
	}
	return o
}
