package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// How many directories to include in category name
const TopParts = 2

type Package struct {
	Path    string
	Top     string
	Base    string
	Name    string
	Imports int
	Lines   int
	Files   int
}

type ClocOutput struct {
	Go ClocGo
}

type ClocGo struct {
	NFiles int
	Code   int
}

type TemplateData struct {
	Packages []*Package
}

// Cloc runs the cloc program on a directory
func Cloc(path string) (*ClocOutput, error) {
	out, err := exec.Command("cloc", "--json", path).Output()
	if err != nil {
		return nil, fmt.Errorf("exec: %v", err)
	}

	c := &ClocOutput{}
	err = json.Unmarshal(out, c)
	if err != nil {
		return nil, fmt.Errorf("unmarshal(%s): %v", out, err)
	}
	return c, nil
}

// PackageName returns the Go package name for a directory
func PackageName(path string) (string, error) {
	c := exec.Command("go", "list")
	c.Dir = path
	c.Env = append(os.Environ(), "GO111MODULE=on")
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("exec: %v", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ProcessDir processes a directory containing Go files
func ProcessDir(root string, path string, imap map[string][]string) (*Package, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return nil, fmt.Errorf("rel(%s, %s): %v", root, path, err)
	}

	pkg, err := PackageName(path)
	if err != nil {
		panic(fmt.Sprintf("PackageName: %v", err))
	}
	c, err := Cloc(path)
	if err != nil {
		panic(fmt.Sprintf("Cloc: %v", err))
	}
	top := strings.Join(strings.Split(rel, "/")[:TopParts], "/")

	p := &Package{
		Path:    path,
		Name:    pkg,
		Base:    filepath.Base(path),
		Top:     top,
		Lines:   c.Go.Code,
		Files:   c.Go.NFiles,
		Imports: len(imap[pkg]),
	}
	return p, nil
}

func parseImports(fset *token.FileSet, path string) ([]string, error) {
	imports := []string{}
	src, err := ioutil.ReadFile(path)
	if err != nil {
		return imports, err
	}

	f, err := parser.ParseFile(fset, "", src, parser.ImportsOnly)
	if err != nil {
		return imports, err
	}

	// Print the imports from the file's AST.
	for _, s := range f.Imports {
		imports = append(imports, strings.Trim(s.Path.Value, `"`))
	}
	return imports, nil
}

func main() {
	fset := token.NewFileSet() // positions are relative to fset
	importMap := map[string][]string{}
	goDirs := map[string]bool{}
	root := os.Args[1]
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".go" {
			imports, err := parseImports(fset, path)
			if err != nil {
				return err
			}

			for _, i := range imports {
				_, ok := importMap[i]
				if !ok {
					importMap[i] = []string{}
				}
				importMap[i] = append(importMap[i], path)
			}
			d := filepath.Dir(path)
			goDirs[d] = true
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Walk(%s): %v", root, err)
	}

	packages := []*Package{}
	for d := range goDirs {
		p, err := ProcessDir(root, d, importMap)
		if err != nil {
			panic(fmt.Sprintf("Rel: %v", err))
		}
		packages = append(packages, p)
	}

	tmpl := template.Must(template.ParseFiles("gopkgsize.tmpl"))
	err = tmpl.Execute(os.Stdout, &TemplateData{Packages: packages})
	if err != nil {
		panic(err)
	}
}
