// Copyright 2018 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package build

import (
	"log"
	"sort"
	"strconv"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/errors"
	"cuelang.org/go/cue/token"
)

type LoadFunc func(path string) *Instance

func (inst *Instance) complete() error {
	// TODO: handle case-insensitive collisions.
	// dir := inst.Dir
	// names := []string{}
	// for _, src := range sources {
	// 	names = append(names, src.path)
	// }
	// f1, f2 := str.FoldDup(names)
	// if f1 != "" {
	// 	return nil, fmt.Errorf("case-insensitive file name collision: %q and %q", f1, f2)
	// }

	var (
		c        = inst.ctxt
		fset     = c.FileSet()
		imported = map[string][]token.Position{}
	)

	for _, f := range inst.Files {
		for _, decl := range f.Decls {
			d, ok := decl.(*ast.ImportDecl)
			if !ok {
				continue
			}
			for _, spec := range d.Specs {
				quoted := spec.Path.Value
				path, err := strconv.Unquote(quoted)
				if err != nil {
					// TODO: remove panic
					log.Panicf("%s: parser returned invalid quoted string: <%s>", f.Filename, quoted)
				}
				imported[path] = append(imported[path], fset.Position(spec.Pos()))
			}
		}
	}

	paths := make([]string, 0, len(imported))
	for path := range imported {
		paths = append(paths, path)
		if path == "" {
			return errors.E(imported[path], "empty import path")
		}
	}

	sort.Strings(paths)

	if inst.loadFunc != nil {
		for i, path := range paths {
			isLocal := IsLocalImport(path)
			if isLocal {
				// path = dirToImportPath(filepath.Join(dir, path))
			}

			imp := c.imports[path]
			if imp == nil {
				imp = inst.loadFunc(path)
				if imp == nil {
					continue
				}
				if imp.Err != nil {
					if len(imported[path]) > 0 {
						imp.Err = errors.Augment(imp.Err, imported[path][0])
					}
					return imp.Err
				}
				imp.ImportPath = path
				// imp.parent = inst
				c.imports[path] = imp
				// imp.parent = nil
			} else if imp.parent != nil {
				// TODO: report a standard cycle message.
				//       cycle is now handled explicitly in loader
			}
			paths[i] = imp.ImportPath

			inst.addImport(imp)
			if imp.Incomplete {
				inst.Incomplete = true
			}
		}
	}

	inst.ImportPaths = paths
	inst.ImportPos = imported

	// Build full dependencies
	deps := make(map[string]*Instance)
	var q []*Instance
	q = append(q, inst.Imports...)
	for i := 0; i < len(q); i++ {
		p1 := q[i]
		path := p1.ImportPath
		// The same import path could produce an error or not,
		// depending on what tries to import it.
		// Prefer to record entries with errors, so we can report them.
		// p0 := deps[path]
		// if err0, err1 := lastError(p0), lastError(p1); p0 == nil || err1 != nil && (err0 == nil || len(err0.ImportStack) > len(err1.ImportStack)) {
		// 	deps[path] = p1
		// 	for _, p2 := range p1.Imports {
		// 		if deps[p2.ImportPath] != p2 {
		// 			q = append(q, p2)
		// 		}
		// 	}
		// }
		if _, ok := deps[path]; !ok {
			deps[path] = p1
		}
	}
	inst.Deps = make([]string, 0, len(deps))
	for dep := range deps {
		inst.Deps = append(inst.Deps, dep)
	}
	sort.Strings(inst.Deps)

	for _, dep := range inst.Deps {
		p1 := deps[dep]
		if p1 == nil {
			panic("impossible: missing entry in package cache for " + dep + " imported by " + inst.ImportPath)
		}
		if p1.Err != nil {
			inst.DepsErrors = append(inst.DepsErrors, p1.Err)
		}
	}

	return nil
}
