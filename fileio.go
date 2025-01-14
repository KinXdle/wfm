package main

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gabriel-vasile/mimetype"
)

func deniedPfx(pfx string) bool {
	cPfx := filepath.Clean(pfx)
	for _, p := range denyPfxs {
		if strings.HasPrefix(cPfx, p) {
			return true
		}
	}
	return false
}

func dispFile(w http.ResponseWriter, uFilePath string) {
	if deniedPfx(uFilePath) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	fp := filepath.Clean(uFilePath)
	s := strings.Split(fp, ".")
	log.Printf("Dsiposition file=%v ext=%v", fp, s[len(s)-1])
	switch strings.ToLower(s[len(s)-1]) {
	case "url", "desktop", "webloc":
		gourl(w, fp)

	case "zip":
		listZip(w, fp)
	case "7z":
		list7z(w, fp)
	case "tar", "rar", "gz", "bz2", "xz", "tgz", "tbz2", "txz":
		listArchive(w, fp)
	case "iso":
		listIso(w, fp)

	default:
		dispInline(w, fp)
	}
}

func downFile(w http.ResponseWriter, uFilePath string) {
	if deniedPfx(uFilePath) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	f, err := os.Stat(uFilePath)
	if err != nil {
		htErr(w, "Unable to get file attributes", err)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(uFilePath)+"\";")
	w.Header().Set("Content-Length", fmt.Sprint(f.Size()))
	w.Header().Set("Cache-Control", *cacheCtl)
	streamFile(w, uFilePath)
}

func dispInline(w http.ResponseWriter, uFilePath string) {
	if deniedPfx(uFilePath) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	f, err := os.Stat(uFilePath)
	if err != nil {
		htErr(w, "Unable to get file attributes", err)
		return
	}

	fi, err := os.Open(uFilePath)
	if err != nil {
		htErr(w, "Unable top open file", err)
		return
	}
	mt, err := mimetype.DetectReader(fi)
	if err != nil {
		htErr(w, "Unable to determine file type", err)
		return
	}
	fi.Close()

	w.Header().Set("Content-Type", mt.String())
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Length", fmt.Sprint(f.Size()))
	w.Header().Set("Cache-Control", *cacheCtl)
	streamFile(w, uFilePath)
}

func streamFile(w http.ResponseWriter, uFilePath string) {
	if deniedPfx(uFilePath) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	fi, err := os.Open(uFilePath)
	if err != nil {
		htErr(w, "Unable top open file", err)
		log.Printf("unable to read file: %v", err)
		return
	}
	defer fi.Close()

	rb := bufio.NewReader(fi)
	wb := bufio.NewWriter(w)
	bu := make([]byte, 1<<20)

	for {
		n, err := rb.Read(bu)
		if err != nil && err != io.EOF {
			htErr(w, "Unable to read file", err)
			log.Printf("unable to read file: %v", err)
			return
		}
		if n == 0 {
			break
		}
		wb.Write(bu[:n])
	}
	wb.Flush()
}

func uploadFile(w http.ResponseWriter, uDir, eSort string, h *multipart.FileHeader, f multipart.File, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	defer f.Close()

	o, err := os.OpenFile(uDir+"/"+filepath.Base(h.Filename), os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		htErr(w, "unable to write file", err)
		return
	}
	defer o.Close()
	rb := bufio.NewReader(f)
	wb := bufio.NewWriter(o)
	bu := make([]byte, 1<<20)

	for {
		n, err := rb.Read(bu)
		if err != nil && err != io.EOF {
			htErr(w, "Unable to write file", err)
			return
		}
		if n == 0 {
			break
		}
		wb.Write(bu[:n])
	}
	wb.Flush()
	log.Printf("Uploaded Dir=%v File=%v Size=%v", uDir, h.Filename, h.Size)
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(h.Filename))
}

func saveText(w http.ResponseWriter, uDir, eSort, uFilePath, uData string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	if uData == "" {
		htErr(w, "text save", fmt.Errorf("zero lenght data"))
		return
	}
	err := ioutil.WriteFile(uFilePath+".tmp", []byte(uData), 0644)
	if err != nil {
		htErr(w, "text save", err)
		return
	}
	f, err := os.Stat(uFilePath + ".tmp")
	if err != nil {
		htErr(w, "text save", err)
		return
	}
	if f.Size() != int64(len(uData)) {
		htErr(w, "text save", fmt.Errorf("temp file size != input size"))
		return
	}
	err = os.Rename(uFilePath+".tmp", uFilePath)
	if err != nil {
		htErr(w, "text save", err)
		return
	}
	log.Printf("Saved Text Dir=%v File=%v Size=%v", uDir, uFilePath, len(uData))
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(filepath.Base(uFilePath)))
}

func mkdir(w http.ResponseWriter, uDir, uNewd, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}

	if uNewd == "" {
		htErr(w, "mkdir", fmt.Errorf("directory name is empty"))
		return
	}
	uB := filepath.Base(uNewd)
	err := os.Mkdir(uDir+"/"+uB, 0755)
	if err != nil {
		htErr(w, "mkdir", err)
		log.Printf("mkdir error: %v", err)
		return
	}
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(uB))
}

func mkfile(w http.ResponseWriter, uDir, uNewf, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}

	if uNewf == "" {
		htErr(w, "mkfile", fmt.Errorf("file name is empty"))
		return
	}
	fB := filepath.Base(uNewf)
	f, err := os.OpenFile(uDir+"/"+fB, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0644)
	if err != nil {
		htErr(w, "mkfile", err)
		return
	}
	f.Close()
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(fB))
}

func mkurl(w http.ResponseWriter, uDir, uNewu, eUrl, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}
	if uNewu == "" {
		htErr(w, "mkurl", fmt.Errorf("url file name is empty"))
		return
	}
	if !strings.HasSuffix(uNewu, ".url") {
		uNewu = uNewu + ".url"
	}
	fB := filepath.Base(uNewu)
	f, err := os.OpenFile(uDir+"/"+fB, os.O_RDWR|os.O_EXCL|os.O_CREATE, 0644)
	if err != nil {
		htErr(w, "mkfile", err)
		return
	}
	// TODO(tenox): add upport for creating webloc, desktop and other formats
	fmt.Fprintf(f, "[InternetShortcut]\r\nURL=%s\r\n", eUrl)
	f.Close()
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(fB))
}

func renFile(w http.ResponseWriter, uDir, uBn, uNewf, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}

	if uBn == "" || uNewf == "" {
		htErr(w, "rename", fmt.Errorf("filename is empty"))
		return
	}
	fB := filepath.Base(uNewf)
	err := os.Rename(
		uDir+"/"+uBn,
		uDir+"/"+fB,
	)
	if err != nil {
		htErr(w, "rename", err)
		return
	}
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort+"&hi="+url.QueryEscape(fB))
}

func moveFiles(w http.ResponseWriter, uDir string, uFilePaths []string, uDst, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) || deniedPfx(uDst) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}

	lF := ""
	for _, f := range uFilePaths {
		fb := filepath.Base(f)
		err := os.Rename(
			uDir+"/"+fb,
			filepath.Clean(uDst+"/"+fb),
		)
		if err != nil {
			htErr(w, "move", err)
			return
		}
		lF = fb
	}
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDst)+"&sort="+eSort+"&hi="+url.QueryEscape(lF))
}

func deleteFiles(w http.ResponseWriter, uDir string, uFilePaths []string, eSort string, rw bool) {
	if !rw {
		htErr(w, "permission", fmt.Errorf("read only"))
		return
	}
	if deniedPfx(uDir) {
		htErr(w, "access", fmt.Errorf("forbidden"))
		return
	}

	for _, f := range uFilePaths {
		err := os.RemoveAll(uDir + "/" + filepath.Base(f))
		if err != nil {
			htErr(w, "delete", err)
			return
		}
	}
	redirect(w, *wfmPfx+"?dir="+url.QueryEscape(uDir)+"&sort="+eSort)
}
