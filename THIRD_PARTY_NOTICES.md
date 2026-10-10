# Third-party notices

UCSPEC's own MIT license does not replace the licenses of its dependencies.
No external font files are distributed; the HTML uses fonts provided by the viewer's system.

Direct dependencies are pinned in go.mod:

- github.com/d2lang/elk-go v0.2.0: ELK placement. Upstream license: Eclipse Public License 2.0. Source: https://github.com/d2lang/elk-go/tree/v0.2.0
- github.com/santhosh-tekuri/jsonschema/v6 v6.0.3: JSON Schema validation. Apache License 2.0. Source: https://github.com/santhosh-tekuri/jsonschema/tree/v6.0.3
- go.yaml.in/yaml/v3 v3.0.4: YAML parser. See the LICENSE/NOTICE files supplied by the exact module. Source and version: https://pkg.go.dev/go.yaml.in/yaml/v3@v3.0.4

The source-only initial archive does not vendor these dependencies or claim to include their full source. Their code is obtained by Go's module system. No dependency code has been copied or modified in this initial source delivery.

Before binary distribution, scripts/build.ps1 collects LICENSE/COPYING/NOTICE files from every downloaded module into dist/third-party, records the versions, and provides module-source download locations in dist/SOURCES.txt. scripts/package.ps1 includes these with the binary. Review the resulting notices and source-availability obligations before publishing. The initial environment could not retrieve the dependencies, so a binary package with these notices has not yet been created.

Runtime/toolchain license is also collected from GOROOT/LICENSE. Build metadata is recorded in BUILD-INFO.txt. Retain all third-party notices when redistributing binary archives.
