package genre

import (
	"archive/zip"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Mede quantos gêneros e quantos livros sobrevivem a cada limiar de frequência mínima.
// A cauda longa é o que torna o filtro inútil: 2198 opções numa lista suspensa não
// filtram nada, por mais corretos que os rótulos estejam.
func TestLimiarDeFrequencia(t *testing.T) {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, "kavita/data/Livros")
	if _, err := os.Stat(root); err != nil {
		t.Skip("biblioteca ausente")
	}
	var files []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".epub") {
			files = append(files, p)
		}
		return nil
	})

	perBook := make([][]string, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 10)
	for _, f := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			z, err := zip.OpenReader(f)
			if err != nil {
				return
			}
			defer z.Close()
			for _, e := range z.File {
				if !strings.HasSuffix(strings.ToLower(e.Name), ".opf") {
					continue
				}
				rc, _ := e.Open()
				if rc == nil {
					break
				}
				buf := make([]byte, 96*1024)
				n, _ := rc.Read(buf)
				rc.Close()
				var raw []string
				for _, m := range reSubj.FindAllStringSubmatch(string(buf[:n]), -1) {
					raw = append(raw, strings.TrimSpace(html.UnescapeString(m[1])))
				}
				if c := Clean(raw); len(c) > 0 {
					mu.Lock()
					perBook = append(perBook, c)
					mu.Unlock()
				}
				break
			}
		}(f)
	}
	wg.Wait()

	freq := map[string]int{}
	for _, gs := range perBook {
		for _, g := range gs {
			freq[g]++
		}
	}
	t.Logf("livros com gênero: %d | vocabulário limpo: %d", len(perBook), len(freq))
	t.Log("")
	t.Logf("%-8s %-12s %-14s %s", "limiar", "gêneros", "livros c/ 1+", "livros que perdem tudo")
	for _, thr := range []int{1, 2, 3, 5, 10, 20} {
		kept := map[string]bool{}
		for g, n := range freq {
			if n >= thr {
				kept[g] = true
			}
		}
		comAlgum, semNada := 0, 0
		for _, gs := range perBook {
			n := 0
			for _, g := range gs {
				if kept[g] {
					n++
				}
			}
			if n > 0 {
				comAlgum++
			} else {
				semNada++
			}
		}
		t.Logf("%-8d %-12d %-14d %d", thr, len(kept), comAlgum, semNada)
	}

	kept := []string{}
	for g, n := range freq {
		if n >= 5 {
			kept = append(kept, g)
		}
	}
	sort.Slice(kept, func(i, j int) bool { return freq[kept[i]] > freq[kept[j]] })
	t.Log("")
	t.Logf("com limiar 5, o vocabulário fica assim (%d gêneros):", len(kept))
	var line []string
	for _, g := range kept {
		line = append(line, g)
	}
	t.Log("    " + strings.Join(line, " · "))
}
