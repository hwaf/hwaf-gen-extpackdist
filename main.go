package main

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hwaf/hwaf/hwaflib"
)

var g_ctx *hwaflib.Context
var g_out = flag.String("o", "packs", "output directory for tarballs")
var g_pkglist = make([]string, 0)

//var g_ignore = flag.String("ignore", ".svn", "comma-separated list of path names to exclude")

var g_ignore = []string{
	".svn",
}

func str_in_slice(str string, slice []string) bool {
	for _, s := range slice {
		if str == s {
			return true
		}
	}
	return false
}

func handle_err(err error) {
	if err != nil {
		panic(fmt.Errorf("hwaf-gen-extpackdist: %v", err))
	}
}

func path_exists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

// commonpath returns the largest commonpath between p1 and p2
//  Example: /foo/bar/include /foo/baz => /foo
func commonpath(p1, p2 string) string {
	if len(p1) < len(p2) {
		return commonpath(p2, p1)
	}
	// len(p1) > len(p2)
	for i := 1; i < len(p2); i++ {
		if p1[:i] != p2[:i] {
			return p1[:i-1]
		}
	}
	return p1
}

func select_pkg(pkg Package) bool {
	if len(g_pkglist) > 0 {
		return str_in_slice(pkg.Name, g_pkglist)
	}
	return true
}

type Package struct {
	Name    string
	Version string
	Root    string
	Variant string
	Dirs    []string
}

func pack(pkg Package) (string, error) {
	var err error
	dirs := pkg.Dirs
	rootdir := pkg.Root

	fmt.Printf(":: packing [%s]... (%s)\n", pkg.Name, rootdir)
	fname := filepath.Join(
		*g_out,
		fmt.Sprintf("%s-%s-%s.tar.gz", pkg.Name, pkg.Version, pkg.Variant),
	)

	f, err := os.Create(fname)
	if err != nil {
		return fname, err
	}
	defer f.Close()

	prefix := fmt.Sprintf("%s-%s", pkg.Name, pkg.Version)

	zout := gzip.NewWriter(f)
	tw := tar.NewWriter(zout)

	for _, dir := range dirs {
		//fmt.Printf(">>> [%s]...\n", dir)
		workdir := dir
		err = filepath.Walk(workdir, func(path string, fi os.FileInfo, err error) error {
			if !strings.HasPrefix(path, workdir) {
				err = fmt.Errorf("walked filename %q doesn't begin with workdir %q", path, workdir)
				return err

			}
			if len(path) == 0 {
				return fmt.Errorf("empty path !!!")
			}

			//fmt.Printf("--- [%s]...\n", path)
			if str_in_slice(filepath.Base(path), g_ignore) {
				//fmt.Printf("--- [%s]... [IGNORED]\n", path)
				return filepath.SkipDir
			}

			name := path
			if strings.HasPrefix(path, rootdir) {
				name = path[len(rootdir):] //path
			} else {
				name = path[len(commonpath(rootdir, path)):] // extract most common path
			}

			// make name "relative"
			if strings.HasPrefix(name, "/") {
				name = name[1:]
			}
			//fmt.Printf("--- [%s]...\n", name)

			target, _ := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr, err := tar.FileInfoHeader(fi, target)
			if err != nil {
				return err
			}
			hdr.Name = filepath.Join(prefix, name)
			hdr.Uname = "root"
			hdr.Gname = "root"
			hdr.Uid = 0
			hdr.Gid = 0

			// Force permissions to 0755 for executables, 0644 for everything else.
			if fi.Mode().Perm()&0111 != 0 {
				hdr.Mode = hdr.Mode&^0777 | 0755
			} else {
				hdr.Mode = hdr.Mode&^0777 | 0644
			}

			err = tw.WriteHeader(hdr)
			if err != nil {
				return fmt.Errorf("Error writing file %q: %v", name, err)
			}
			// handle directories and symlinks
			if hdr.Size <= 0 {
				return nil
			}
			r, err := os.Open(path)
			if err != nil {
				return err
			}
			defer r.Close()
			_, err = io.Copy(tw, r)
			return err
		})

		if err != nil {
			return fname, err
		}
	}

	if err := tw.Close(); err != nil {
		return fname, err
	}

	if err := zout.Close(); err != nil {
		return fname, err
	}

	return fname, f.Close()
}

func main() {
	var err error

	flag.Parse()
	if *g_out == "" || *g_out == "." {
		*g_out, err = os.Getwd()
		handle_err(err)
	}
	if !path_exists(*g_out) {
		err = os.MkdirAll(*g_out, 0755)
		if err != nil {
			fmt.Fprintf(os.Stderr, "**error**: could not create directory [%s]: %v\n", *g_out, err)
		}
		handle_err(err)
	}

	g_pkglist = flag.Args()

	g_ctx, err = hwaflib.NewContext()
	handle_err(err)

	pwd, err := os.Getwd()
	handle_err(err)

	g_ctx, err = hwaflib.NewContextFrom(pwd)
	handle_err(err)

	pinfo, err := g_ctx.ProjectInfos()
	handle_err(err)

	all_good := true

	variant := g_ctx.Variant()

	all_keys := pinfo.Keys()
	pkgs := make([]Package, 0, 32)
	for _, k := range all_keys {
		if !strings.HasSuffix(k, "_export_paths") {
			continue
		}
		name := k[:len(k)-len("_export_paths")]
		pkg := Package{Name: name}
		if !select_pkg(pkg) {
			continue
		}

		rootdir, err := pinfo.Get(name + "_home")
		if err != nil {
			fmt.Fprintf(os.Stderr, "**err** %v\n", err)
			all_good = false
			continue
		}
		//fmt.Printf("=========== [%s] ============\n", name)
		version, err := pinfo.Get(name + "_native_version")
		handle_err(err)

		if strings.HasPrefix(version, name+"-") {
			version = version[len(name+"-"):]
		}
		if strings.HasPrefix(version, name+"_") {
			version = version[len(name+"_"):]
		}

		pkg = Package{
			Name:    name,
			Version: version,
			Root:    rootdir,
			Variant: variant,
			Dirs:    make([]string, 0),
		}
		val := make([]string, 0)
		raw_val, err := pinfo.Get(k)
		handle_err(err)

		if strings.HasPrefix(raw_val, `'`) {
			tmp_val := strings.Split(raw_val, `', `)
			for _, v := range tmp_val {
				v = strings.TrimSpace(v)
				if len(v) == 0 {
					continue
				}
				ibeg := len(`'`)
				iend := len(v)
				if strings.HasSuffix(v, `'`) {
					iend = len(v) - len(`'`)
				}
				vv := v[ibeg:iend]
				if vv != "" {
					//fmt.Printf("--> %d:%d |%s| => |%s|\n", ibeg, iend, v, v[ibeg:iend])
					val = append(val, v[ibeg:iend])
				}
			}
		} else if strings.HasPrefix(raw_val, `['`) {
			tmp_val := strings.Split(raw_val, `', `)
			for _, v := range tmp_val {
				v = strings.TrimSpace(v)
				if len(v) == 0 {
					continue
				}
				ibeg := len(`'`)
				if strings.HasPrefix(v, `['`) {
					ibeg = len(`['`)
				}
				iend := len(v)
				if strings.HasSuffix(v, `']`) {
					iend = len(v) - len(`']`)
				}
				vv := v[ibeg:iend]
				if vv != "" {
					//fmt.Printf("--> %d:%d |%s| => |%s|\n", ibeg, iend, v, v[ibeg:iend])
					val = append(val, v[ibeg:iend])
				}
			}
		} else {
			if raw_val != "" {
				val = append(val, raw_val)
			}
		}
		for _, dir := range val {
			if !path_exists(dir) {
				fmt.Fprintf(os.Stderr, "*** warn: [%s] no such directory [%s]\n", pkg.Name, dir)
				continue
			}
			pkg.Dirs = append(pkg.Dirs, dir)
		}

		if len(pkg.Dirs) == 0 {
			fmt.Printf("*** warn: empty dirs for pack [%s]\n", pkg.Name)
			continue
		}
		if select_pkg(pkg) {
			pkgs = append(pkgs, pkg)
		}

		//fmt.Printf("--> [%s-%s] => %v (%v)\n", pkg.Name, pkg.Version, val, raw_val)
	}

	var wg sync.WaitGroup
	wg.Add(len(pkgs))

	for _, pkg := range pkgs {
		go func(pkg Package) {
			fname, err := pack(pkg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "*** error creating pack [%s]: %v\n", pkg.Name, err)
				all_good = false
			}
			fmt.Printf("=> %s\n", fname)
			fmt.Printf(":: packing [%s]... (%s) [done]\n", pkg.Name, pkg.Root)
			wg.Done()
		}(pkg)
	}

	wg.Wait()

	if !all_good {
		os.Exit(1)
	}
	os.Exit(0)
}

// EOF
