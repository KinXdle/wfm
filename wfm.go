// Web File Manager

package main

import (
	"crypto/tls"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"syscall"

	_ "github.com/breml/rootcerts"
	"golang.org/x/crypto/acme/autocert"
)

type multiString []string

var (
	vers        = "2.0.2"
	bindProto   = flag.String("proto", "tcp", "tcp, tcp4, tcp6, etc")
	bindAddr    = flag.String("addr", "127.0.0.1:8080", "Listen address, eg: :443")
	bindExtra   = flag.String("addr_extra", "", "Extra non-TLS listener address, eg: :8081")
	chrootDir   = flag.String("chroot", "", "Directory to chroot to")
	suidUser    = flag.String("setuid", "", "Username to setuid to")
	allowRoot   = flag.Bool("allow_root", false, "allow to run as uid=0/root without setuid")
	logFile     = flag.String("logfile", "", "Log file name (default stdout)")
	passwdDb    = flag.String("passwd", "", "wfm password file, eg: /usr/local/etc/wfmpw.json")
	noPwdDbRW   = flag.Bool("nopass_rw", false, "allow read-write access if there is no password file")
	aboutRnt    = flag.Bool("about_runtime", true, "Display runtime info in About Dialog")
	showDot     = flag.Bool("show_dot", false, "show dot files and folders")
	wfmPfx      = flag.String("prefix", "/", "Default prefix for WFM access")
	docSrv      = flag.String("doc_srv", "", "Serve regular http files, fsdir:prefix, eg /var/www:/home")
	cacheCtl    = flag.String("cache_ctl", "no-cache", "HTTP Header Cache Control")
	acmDir      = flag.String("acm_dir", "", "autocert cache, eg: /var/cache (inside chroot)")
	acmBind     = flag.String("acm_addr", "", "autocert manager listen address, eg: :80")
	acmWhlist   multiString // this flag set in main
	denyPfxs    multiString
	allowAcmDir = flag.Bool("allow_acm_dir", false, "allow access to acm cache dir (insecure!)")
	f2bEnabled  = flag.Bool("f2b", true, "ban ip addresses on user/pass failures")
	f2bDump     = flag.String("f2b_dump", "", "enable f2b dump at this prefix, eg. /f2bdump (default no)")
)

func userId(usr string) (int, int, error) {
	u, err := user.Lookup(usr)
	if err != nil {
		return 0, 0, err
	}
	ui, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gi, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return ui, gi, nil
}

func setUid(ui, gi int) error {
	if ui == 0 || gi == 0 {
		return nil
	}
	err := syscall.Setgid(gi)
	if err != nil {
		return err
	}
	err = syscall.Setuid(ui)
	if err != nil {
		return err
	}
	return nil
}

func (z *multiString) String() string {
	return "something"
}

func (z *multiString) Set(v string) error {
	*z = append(*z, v)
	return nil
}

func main() {
	var err error
	flag.Var(&acmWhlist, "acm_host", "autocert manager allowed hostname (multi)")
	flag.Var(&denyPfxs, "deny_pfx", "deny access / hide this path prefix (multi)")
	flag.Parse()

	if flag.Arg(0) == "user" {
		manageUsers()
		return
	}

	log.Print("WFM Starting up")

	if *passwdDb != "" {
		loadUsers()
	}

	if !*allowAcmDir && *acmDir != "" {
		denyPfxs = append(denyPfxs, *acmDir)
	}

	if *logFile != "" {
		lf, err := os.OpenFile(*logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		defer lf.Close()
		log.SetOutput(lf)
	}

	// find uid/gid for setuid before chroot
	var suid, sgid int
	if *suidUser != "" {
		suid, sgid, err = userId(*suidUser)
		if err != nil {
			log.Fatal("unable to find setuid user", err)
		}
	}

	// run autocert manager before chroot/setuid
	// however it doesn't matter for chroot as certs will land in chroot *adir anyway
	acm := autocert.Manager{}
	if *bindAddr != "" && *acmDir != "" && len(acmWhlist) > 0 {
		acm.Prompt = autocert.AcceptTOS
		acm.Cache = autocert.DirCache(*acmDir)
		acm.HostPolicy = autocert.HostWhitelist(acmWhlist...)
		go http.ListenAndServe(*acmBind, acm.HTTPHandler(nil))
		log.Printf("Autocert enabled for %v", acmWhlist)
	}

	// chroot now
	if *chrootDir != "" {
		err := syscall.Chroot(*chrootDir)
		if err != nil {
			log.Fatal("chroot", err)
		}
		log.Printf("Chroot to %q", *chrootDir)
	}

	// listen/bind to port before setuid
	l, err := net.Listen(*bindProto, *bindAddr)
	if err != nil {
		log.Fatalf("unable to listen on %v: %v", *bindAddr, err)
	}
	log.Printf("Listening on %q", *bindAddr)

	// setuid now
	err = setUid(suid, sgid)
	if err != nil {
		log.Fatalf("unable to suid for %v: %v", *suidUser, err)
	}
	if !*allowRoot && os.Getuid() == 0 {
		log.Fatal("you probably dont want to run wfm as root, use --allow_root flag to force it")
	}
	log.Printf("Setuid UID=%d GID=%d", os.Geteuid(), os.Getgid())

	// http stuff
	mux := http.NewServeMux()
	mux.HandleFunc(*wfmPfx, wfm)
	mux.HandleFunc("/favicon.ico", favicon)
	if *f2bDump != "" {
		mux.HandleFunc(*f2bDump, dumpf2b)
	}
	if *docSrv != "" {
		ds := strings.Split(*docSrv, ":")
		log.Printf("Starting doc handler for dir %v at %v", ds[0], ds[1])
		mux.Handle(ds[1], http.StripPrefix(ds[1], http.FileServer(http.Dir(ds[0]))))
	}

	if *bindExtra != "" {
		log.Printf("Listening (extra) on %q", *bindAddr)
		go http.ListenAndServe(*bindExtra, mux)
	}
	if *bindAddr != "" && *acmDir != "" && len(acmWhlist) > 0 {
		https := &http.Server{
			Addr:      *bindAddr,
			Handler:   mux,
			TLSConfig: &tls.Config{GetCertificate: acm.GetCertificate},
		}
		log.Printf("Starting HTTPS TLS Server")
		err = https.ServeTLS(l, "", "")
	} else {
		log.Printf("Starting HTTP Server")
		err = http.Serve(l, mux)
	}
	if err != nil {
		log.Fatal(err)
	}
}
