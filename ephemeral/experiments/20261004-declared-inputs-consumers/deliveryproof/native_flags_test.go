package deliveryproof

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDeliveryNativeGetterManagerCheckIgnoreDoesNotWrite(t *testing.T) {
	wholeTree := func(root string) map[string]string {
		t.Helper()
		result := map[string]string{}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, path)
			if d.IsDir() {
				result[rel+"/"] = "directory"
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] = sha(string(body))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	for _, operation := range []string{"version", "getter"} {
		for _, ignore := range []bool{false, true} {
			name := operation + "-stock"
			if ignore {
				name = operation + "-ignore"
			}
			t.Run(name, func(t *testing.T) {
				root := filepath.Join(proofRoot, "delivery-fixtures/native-getter-ignore-c512e83", name)
				if _, err := os.Stat(root); !os.IsNotExist(err) {
					t.Fatalf("refuse retained native fixture:%s:%v", root, err)
				}
				write(t, root, "package.json", `{"name":"native-getter-control","private":true,"packageManager":"pnpm@12.9.1","devDependencies":{"@sveltejs/kit":"^3.0.0"}}`)
				absolute := filepath.Join(root, "patches/absolute.patch")
				workspace := "patchedDependencies:\n  independent-relative@1.0.0: patches/relative.patch\n  independent-absolute@1.0.0: " + absolute + "\n"
				write(t, root, "pnpm-workspace.yaml", workspace)
				write(t, root, "patches/relative.patch", "independent relative bytes17\n")
				write(t, root, "patches/absolute.patch", "independent absolute bytes17\n")
				before := wholeTree(root)
				args := []string{"pnpm"}
				if ignore {
					args = append(args, "--pm-on-fail=ignore")
				}
				if operation == "version" {
					args = append(args, "--version")
				} else {
					args = append(args, "config", "get", "patchedDependencies", "--location=project", "--json")
				}
				out, err := deliveryRun(t, root, "../"+name+".log", nil, args...)
				if err != nil {
					t.Fatalf("real native %s:%v:%s", name, err, out)
				}
				if operation == "version" {
					if strings.TrimSpace(out) != "12.9.1" {
						t.Fatalf("native version%q want literal12.9.1", out)
					}
				} else {
					var got map[string]string
					if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &got); err != nil {
						t.Fatalf("native getter JSON:%v:%s", err, out)
					}
					want := map[string]string{"independent-relative@1.0.0": filepath.Join(root, "patches/relative.patch"), "independent-absolute@1.0.0": absolute}
					if !reflect.DeepEqual(got, want) {
						t.Fatalf("native effective map%v want authored/resolved literal%v", got, want)
					}
				}
				if content(t, filepath.Join(root, "pnpm-workspace.yaml")) != workspace {
					t.Fatal("native getter changed raw relative/absolute authored strings")
				}
				after := wholeTree(root)
				if ignore {
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("native ignore mutated complete tree before%v after%v", before, after)
					}
				} else {
					if _, err := os.Stat(filepath.Join(root, "pnpm-lock.yaml")); err != nil {
						t.Fatalf("stock native manager bootstrap negative lost lock write:%v", err)
					}
				}
				t.Logf("native %s same literal version/map; complete tree unchanged=%t; stock manager lock=%t", name, reflect.DeepEqual(after, before), !ignore)
			})
		}
	}
}
