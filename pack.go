package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func select_pkg(pkg Package) bool {
	if len(g_pkglist) > 0 {
		return str_in_slice(pkg.Name, g_pkglist)
	}
	return true
}

type Package struct {
	Name     string
	Version  string
	Root     string
	Variant  string
	Siteroot string
	Dirs     []string
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

func collect_pkgs() ([]Package, error) {

	pinfo, err := g_ctx.ProjectInfos()
	if err != nil {
		return nil, err
	}

	all_good := true
	variant := g_ctx.Variant()
	siteroot, err := pinfo.Get("SITEROOT")
	handle_err(err)

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
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(version, name+"-") {
			version = version[len(name+"-"):]
		}
		if strings.HasPrefix(version, name+"_") {
			version = version[len(name+"_"):]
		}

		pkg = Package{
			Name:     name,
			Version:  version,
			Root:     rootdir,
			Variant:  variant,
			Siteroot: siteroot,
			Dirs:     make([]string, 0),
		}
		val := make([]string, 0)
		raw_val, err := pinfo.Get(k)
		if err != nil {
			return nil, err
		}

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

	if !all_good {
		return nil, fmt.Errorf("problem collecting packages")
	}

	return pkgs, nil
}

func pack_pkgs(pkgs []Package) error {
	var err error

	var wg sync.WaitGroup
	wg.Add(len(pkgs))

	all_good := true
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
		return fmt.Errorf("problem creating a pack")
	}
	return err
}

func unpack(pkg Package, rootdir string) error {
	var err error

	fname := filepath.Join(*g_out, fmt.Sprintf("%s-%s-%s.tar.gz", pkg.Name, pkg.Version, pkg.Variant))
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()

	untar := exec.Command(
		"tar",
		"-C", rootdir,
		// remove the package-version/ directory
		"--strip-components=1",
		"-zxf",
		fname,
	)
	untar.Stdin = os.Stdin
	untar.Stdout = os.Stdout
	untar.Stderr = os.Stderr

	err = untar.Run()
	return err
}

// EOF
