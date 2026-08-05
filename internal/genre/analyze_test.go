package genre

import (
	"archive/zip"
	"html"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
)

var reSubj = regexp.MustCompile(`<dc:subject[^>]*>([^<]+)</dc:subject>`)

// Aplica a limpeza ao vocabulário real da biblioteca e mostra o antes/depois. Não é
// asserção: serve para conferir o efeito antes de escrever em milhares de arquivos.
func TestEfeitoNaBibliotecaReal(t *testing.T) {
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
	all := map[string]int{}
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
				rc, err := e.Open()
				if err != nil {
					continue
				}
				buf := make([]byte, 96*1024)
				n, _ := rc.Read(buf)
				rc.Close()
				mu.Lock()
				for _, m := range reSubj.FindAllStringSubmatch(string(buf[:n]), -1) {
					all[strings.TrimSpace(html.UnescapeString(m[1]))]++
				}
				mu.Unlock()
				break
			}
		}(f)
	}
	wg.Wait()

	st, after := Analyze(all)
	t.Logf("ANTES:  %d gêneros distintos, %d atribuições", st.DistinctBefore, st.Before)
	t.Logf("DEPOIS: %d gêneros distintos, %d atribuições", st.DistinctAfter, st.After)
	t.Logf("removidos: %d rótulos distintos (%d%%), %d atribuições",
		st.DistinctBefore-st.DistinctAfter,
		(st.DistinctBefore-st.DistinctAfter)*100/max(st.DistinctBefore, 1),
		st.Before-st.After)

	type kv struct {
		k string
		v int
	}
	var top []kv
	for k, v := range after {
		top = append(top, kv{k, v})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].v != top[j].v {
			return top[i].v > top[j].v
		}
		return top[i].k < top[j].k
	})
	t.Log("--- 25 gêneros mais comuns depois da limpeza:")
	for i, x := range top {
		if i >= 25 {
			break
		}
		t.Logf("    %5d  %s", x.v, x.k)
	}
	um := 0
	for _, v := range after {
		if v == 1 {
			um++
		}
	}
	t.Logf("--- cauda longa: %d gêneros com 1 livro só (%d%% do vocabulário limpo)",
		um, um*100/max(len(after), 1))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
