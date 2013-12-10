package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
)

func pack_project(pkgs []Package, proj_name, proj_vers string) error {
	var err error
	dir, err := ioutil.TempDir("", "hwaf-gen-pack-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	proj_dir := fmt.Sprintf("%s-%s", proj_name, proj_vers)
	fmt.Printf(">>> pack-dir: [%s/%s]\n", dir, proj_dir)

	//throttle := make(chan struct{}, 4)
	for _, pkg := range pkgs {
		homedir := pkg.Root[len(commonpath(pkg.Root, pkg.Siteroot)):]
		odir := filepath.Join(dir, proj_dir, homedir)
		err = os.MkdirAll(odir, 0755)
		if err != nil {
			return err
		}

		err = unpack(pkg, odir)
		if err != nil {
			return err
		}
	}

	variant := pkgs[0].Variant
	fname := fmt.Sprintf("%s-%s-%s.tar.gz", proj_name, proj_vers, variant)

	fname, err = filepath.Abs(filepath.Join(*g_out, fname))
	if err != nil {
		return err
	}

	fmt.Printf(">>> pack-project...\n")
	tar := exec.Command(
		"tar",
		"zcf",
		fname,
		proj_dir,
	)
	tar.Stdin = os.Stdin
	tar.Stdout = os.Stdout
	tar.Stderr = os.Stderr
	tar.Dir = dir
	err = tar.Run()
	return err
}

// EOF
