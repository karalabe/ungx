// Copyright 2018 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// fork defines an optional import path to rewrite the main package to. It's main
// use is when a gx package is forked into a different repo and avoids having to
// do an extra rewrite after copying the code.
var fork = flag.String("fork", "", "Optional root import path to rewrite to")

func main() {
	flag.Parse()

	// Create a temporary Go workspace to download canonical packages into
	workspace, err := ioutil.TempDir("", "")
	if err != nil {
		log.Fatalf("Failed to create temporary workspace: %v", err)
	}
	defer os.RemoveAll(workspace)

	// Resolve the current package's import path
	root, err := exec.Command("go", "list").CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to resolve package import path: %v", err)
	}
	root = bytes.TrimSpace(root)

	// Retrieve all the gx dependencies into the local vendor folder
	deps := exec.Command("gx", "install", "--local")
	deps.Stdout = os.Stdout
	deps.Stderr = os.Stderr

	log.Printf("Vendoring in gx dependencies")
	if err := deps.Run(); err != nil {
		log.Fatalf("Failed to vendor dependencies: %v", err)
	}
	// Find all the unique import paths (duplicates remain unmodified)
	gxpkgs := filepath.Join("vendor", "gx", "ipfs")

	hashes, err := ioutil.ReadDir(gxpkgs)
	if err != nil {
		log.Fatalf("Failed to list vendored packages: %v", err)
	}
	versions := make(map[string]int)
	mappings := make(map[string]string)

	for _, hash := range hashes {
		// Retrieve the package spec from the dependency
		dirs, err := ioutil.ReadDir(filepath.Join(gxpkgs, hash.Name()))
		if err != nil {
			log.Fatalf("Failed to list package contents: %v", err)
		}
		blob, err := ioutil.ReadFile(filepath.Join(gxpkgs, hash.Name(), dirs[0].Name(), "package.json"))
		if err != nil {
			log.Fatalf("Failed to read package definition: %v", err)
		}
		// Extract the canonical package import path
		var pkg struct {
			Gx struct {
				Path string `json:"dvcsimport"`
			} `json:"gx"`
		}
		if err := json.Unmarshal(blob, &pkg); err != nil {
			log.Fatalf("Failed to parse package definition: %v", err)
		}
		// Save the hash to path mapping and clash count
		mappings[hash.Name()] = pkg.Gx.Path
		versions[pkg.Gx.Path]++
	}
	// Move the package from hash to canonical path
	rewrite := make(map[string]string)

	log.Printf("Converting gx dependencies to canonical paths")
	for hash, path := range mappings {
		// Clashing dependencies cannot be rewritten, so they need to be embedded
		if versions[path] > 1 {
			if err := os.MkdirAll(filepath.Join("gxlibs", "ipfs"), 0700); err != nil {
				log.Fatalf("Failed to create canonical embed path: %v", err)
			}
			log.Printf("Embedding gx/ipfs/%s to gxlibs/ipfs/%s", hash, hash)
			if err := os.Rename(filepath.Join(gxpkgs, hash), filepath.Join("gxlibs", "ipfs", hash)); err != nil {
				log.Fatalf("Failed to move embedded package: %v", err)
			}
			rewrite["gx/ipfs/"+hash] = string(root) + "/gxlibs/ipfs/" + hash

			continue
		}
		// Any gx-based dependency should be embedded directly to allow library reuse
		if shouldEmbed(workspace, path) {
			if err := os.MkdirAll(filepath.Join("gxlibs", filepath.Dir(path)), 0700); err != nil {
				log.Fatalf("Failed to create canonical embed path: %v", err)
			}
			dirs, err := ioutil.ReadDir(filepath.Join(gxpkgs, hash))
			if err != nil {
				log.Fatalf("Failed to list package contents: %v", err)
			}
			for _, dir := range dirs {
				log.Printf("Embedding gx/ipfs/%s/%s to gxlibs/%s", hash, dir.Name(), path)
				if err := os.Rename(filepath.Join(gxpkgs, hash, dir.Name()), filepath.Join("gxlibs", path)); err != nil {
					log.Fatalf("Failed to move embedded package: %v", err)
				}
				rewrite["gx/ipfs/"+hash+"/"+dir.Name()] = string(root) + "/gxlibs/" + path
				rewrite[path] = string(root) + "/gxlibs/" + path
			}
		} else {
			// Non-clashing plain Go dependencies can be vendored in
			if err := os.MkdirAll(filepath.Join("vendor", filepath.Dir(path)), 0700); err != nil {
				log.Fatalf("Failed to create canonical vendor path: %v", err)
			}
			dirs, err := ioutil.ReadDir(filepath.Join(gxpkgs, hash))
			if err != nil {
				log.Fatalf("Failed to list package contents: %v", err)
			}
			for _, dir := range dirs {
				log.Printf("Vendoring gx/ipfs/%s/%s to vendor/%s", hash, dir.Name(), path)
				if err := os.Rename(filepath.Join(gxpkgs, hash, dir.Name()), filepath.Join("vendor", path)); err != nil {
					log.Fatalf("Failed to move vendored package: %v", err)
				}
				rewrite["gx/ipfs/"+hash+"/"+dir.Name()] = path
			}
		}
		// Delete the empty hash dependency path
		if err := os.Remove(filepath.Join(gxpkgs, hash)); err != nil {
			log.Fatalf("Failed to remove gx leftover: %v", err)
		}
	}
	// Rewrite packages to their canonical paths
	log.Printf("Rewriting import statements to canonical paths")
	restrict := regexp.MustCompile(`// import ".*"`)

	if err := filepath.Walk(".", func(fp string, fi os.FileInfo, err error) error {
		// Abort if any error occurred, descend into directories
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return nil
		}
		// Replace the relevant import path in all Go files
		if strings.HasSuffix(fi.Name(), ".go") {
			oldblob, err := ioutil.ReadFile(fp)
			if err != nil {
				return err
			}
			newblob := oldblob
			for gxpath, gopath := range rewrite {
				newblob = bytes.Replace(newblob, []byte("\""+gxpath), []byte("\""+gopath), -1)
			}
			if *fork != "" {
				newblob = bytes.Replace(newblob, []byte("\""+string(root)+"/"), []byte("\""+*fork+"/"), -1)
				newblob = bytes.Replace(newblob, []byte("\""+string(root)+"\""), []byte("\""+*fork+"\""), -1)
			}
			newblob = restrict.ReplaceAll(newblob, []byte{})
			if !bytes.Equal(oldblob, newblob) {
				if err = ioutil.WriteFile(fp, newblob, 0); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		log.Fatalf("Failed to rewrite import paths: %v", err)
	}
}

// shouldEmbed returns whether a package identified by its import path should be
// embedded directly into a ungx-ed package or whether vendoring is enough. The
// deciding factor is whether the package's canonical version is gx based or not,
// since we can't vendor gx packages.
func shouldEmbed(gopath string, path string) bool {
	log.Printf("Deciding whether to vendor or embed %s", path)

	// If the import path points to GitHub, we can cheat and directly decide
	if strings.HasPrefix(path, "github.com/") {
		// Try to retrieve the gx package spec, embed on hard failure
		res, err := http.Get(fmt.Sprintf("https://%s/master/package.json", strings.Replace(path, "github.com", "raw.githubusercontent.com", 1)))
		if err != nil {
			return true
		}
		defer res.Body.Close()

		// If the file exists, assume its a gx based project, otherwise vendor
		return res.StatusCode == http.StatusOK
	}
	// Non-github package or something failed, we need to download the canonical code
	get := exec.Command("go", "get", "-d", path+"/...")
	get.Stdout = os.Stdout
	get.Stderr = os.Stderr
	get.Env = append(os.Environ(), "GOPATH="+gopath)

	if err := get.Run(); err == nil {
		if _, err := os.Stat(filepath.Join(gopath, "src", path, "package.json")); err != nil {
			return false
		}
	}
	return true
}
