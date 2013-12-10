package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hwaf/hwaf/hwaflib"
)

var g_ctx *hwaflib.Context
var g_out = flag.String("o", "packs", "output directory for tarballs")
var g_list = flag.Bool("list", false, "list packages which can be packed (do not build)")
var g_packall = flag.Bool("pack-all", false, "pack all packs into a project-level tarball")
var g_packname = flag.String("pack-name", "", "name of the project-level tarball")
var g_packvers = flag.String("pack-version", "", "version for the project-level tarball")
var g_verbose = flag.Bool("v", false, "enable verbose output")

var g_pkglist = make([]string, 0)

//var g_ignore = flag.String("ignore", ".svn", "comma-separated list of path names to exclude")

var g_ignore = []string{
	".svn",
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

	pkgs, err := collect_pkgs()
	handle_err(err)

	if *g_list {
		for _, pkg := range pkgs {
			fmt.Printf("=> %-20s (version=%s)\n", pkg.Name, pkg.Version)
		}
	} else {

		err = pack_pkgs(pkgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "*** error packing tarball: %v\n", err)
			os.Exit(1)
		}

		proj_name := *g_packname
		proj_vers := *g_packvers

		if *g_packall {
			if *g_packname == "" {
				proj_name, err = pinfo.Get("HWAF_BDIST_APPNAME")
				if err != nil {
					fmt.Fprintf(os.Stderr, "*** error retrieving HWAF_BDIST_APPNAME: %v\n", err)
					os.Exit(1)
				}
			}
			if *g_packvers == "" {
				proj_vers, err = pinfo.Get("HWAF_BDIST_VERSION")
				if err != nil {
					fmt.Fprintf(os.Stderr, "*** error retrieving HWAF_BDIST_VERSION: %v\n", err)
					os.Exit(1)
				}
			}

			fmt.Printf(">>> %s-%s.tar.gz\n", proj_name, proj_vers)
			err = pack_project(pkgs, proj_name, proj_vers)
			if err != nil {
				fmt.Fprintf(os.Stderr, "*** error packing project-level tarball: %v\n", err)
				os.Exit(1)
			}
		}
	}

	os.Exit(0)
}

// EOF
