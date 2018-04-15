// Copyright 2018 Péter Szilágyi. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
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
		if versions[path] > 1 {
			continue
		}
		if err := os.MkdirAll(filepath.Join("vendor", filepath.Dir(path)), 0700); err != nil {
			log.Fatalf("Failed to create canonical path: %v", err)
		}
		dirs, err := ioutil.ReadDir(filepath.Join(gxpkgs, hash))
		if err != nil {
			log.Fatalf("Failed to list package contents: %v", err)
		}
		for _, dir := range dirs {
			log.Printf("Rewriting gx/ipfs/%s/%s to %s", hash, dir.Name(), path)
			if err := os.Rename(filepath.Join(gxpkgs, hash, dir.Name()), filepath.Join("vendor", path)); err != nil {
				log.Fatalf("Failed to move canonical package: %v", err)
			}
			rewrite["gx/ipfs/"+hash+"/"+dir.Name()] = path
		}
		if err := os.Remove(filepath.Join(gxpkgs, hash)); err != nil {
			log.Fatalf("Failed to remote gx leftover: %v", err)
		}
	}
	// Rewrite packages to their canonical paths
	log.Printf("Rewriting import statements to canonical paths")

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
				newblob = bytes.Replace(newblob, []byte(gxpath), []byte(gopath), -1)
			}
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
