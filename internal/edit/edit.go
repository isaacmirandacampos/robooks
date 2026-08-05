// Package edit opera sobre livros que já estão na biblioteca: completa metadados que
// faltam e reaplica o layout do alvo.
//
// A diferença para o ingest é a origem: ingest recebe arquivos de fora, edit trabalha no
// que já está organizado. Um serve para o que chega, o outro para acertar o acervo.
package edit

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isaacmirandacampos/robooks/internal/enrich"
	"github.com/isaacmirandacampos/robooks/internal/epub"
	"github.com/isaacmirandacampos/robooks/internal/genre"
	"github.com/isaacmirandacampos/robooks/internal/meta"
	"github.com/isaacmirandacampos/robooks/internal/target"
)

const (
	pythonBin = "/usr/bin/python3"
	metaBin   = "/usr/bin/ebook-meta"
)

type Options struct {
	Library      string
	Paths        []string // subconjunto opcional; vazio = biblioteca inteira
	Target       target.Target
	Apply        bool
	Enrich       bool
	Relayout     bool
	Workers      int
	Limit        int
	Timeout      time.Duration
	TagsPT       bool
	FailLog      string
	CleanGenres  bool
	MinGenreFreq int
}

type item struct {
	rel  string
	full string
	m    epub.Meta
	b    meta.Book
}

// Run percorre a biblioteca e aplica o que foi pedido.
//
// É retomável por construção: um livro que já tem o campo preenchido é pulado, então
// interromper e rodar de novo continua de onde parou sem precisar de estado extra. Isso
// importa porque completar 10 mil livros leva horas.
func Run(o Options) error {
	if o.Workers <= 0 {
		// Consulta externa usa concorrência baixa de propósito: as fontes bloqueiam
		// quem dispara em rajada, e o gargalo aqui é a rede, não a CPU.
		if o.Enrich {
			o.Workers = 4
		} else {
			o.Workers = max(1, runtime.NumCPU()-2)
		}
	}
	if o.Timeout == 0 {
		o.Timeout = 90 * time.Second
	}
	if o.Enrich && !enrich.Available() {
		return fmt.Errorf("fetch-ebook-metadata não encontrado; instale o calibre para usar -enrich")
	}

	items, err := collect(o)
	if err != nil {
		return err
	}

	fmt.Printf("biblioteca: %s\n", o.Library)
	fmt.Printf("alvo:       %s\n", o.Target.Name())
	fmt.Printf("candidatos: %d livro(s)\n", len(items))
	if o.Enrich {
		fmt.Printf("enrich:     ligado, %d consultas simultâneas, timeout %s\n", o.Workers, o.Timeout)
		est := time.Duration(len(items)/o.Workers*12) * time.Second
		fmt.Printf("            estimativa: ~%s (12s por consulta é a média medida)\n", est.Round(time.Minute))
	}
	if o.Relayout {
		fmt.Printf("relayout:   ligado — arquivos serão movidos para o layout de %s\n", o.Target.Name())
	}
	fmt.Println()

	if len(items) == 0 {
		fmt.Println("nada a fazer: todos os livros já têm os campos pedidos.")
		return nil
	}
	if o.CleanGenres {
		return cleanGenres(o)
	}
	if !o.Apply {
		preview(o, items)
		return nil
	}
	return apply(o, items)
}

// cleanGenres reescreve os gêneros da biblioteca inteira.
//
// São duas passadas por necessidade, não por desleixo: decidir se um gênero fica
// depende de quantos livros o usam, e isso só se sabe depois de ler todos. A primeira
// passada monta o vocabulário; a segunda escreve.
func cleanGenres(o Options) error {
	var files []string
	roots := o.Paths
	if len(roots) == 0 {
		roots = []string{o.Library}
	}
	for _, root := range roots {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
			if e == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(d.Name()), ".epub") &&
				!strings.HasPrefix(d.Name(), ".") {
				files = append(files, p)
			}
			return nil
		})
	}

	type bookTags struct {
		path  string
		antes []string
		limpo []string
	}
	all := make([]bookTags, 0, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, runtime.NumCPU()-2))
	for _, f := range files {
		wg.Add(1)
		go func(f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m, err := epub.ReadMeta(f)
			if err != nil || len(m.Tags) == 0 {
				return
			}
			c := genre.Clean(m.Tags)
			mu.Lock()
			all = append(all, bookTags{f, m.Tags, c})
			mu.Unlock()
		}(f)
	}
	wg.Wait()

	perBook := make([][]string, 0, len(all))
	for _, b := range all {
		perBook = append(perBook, b.limpo)
	}
	vocab := genre.BuildVocabulary(perBook, o.MinGenreFreq)

	antesDistintos := map[string]bool{}
	for _, b := range all {
		for _, t := range b.antes {
			antesDistintos[t] = true
		}
	}

	var mudam, esvaziam int
	for i := range all {
		final := vocab.Apply(all[i].antes)
		if !sameSet(final, all[i].antes) {
			mudam++
		}
		if len(final) == 0 && len(all[i].antes) > 0 {
			esvaziam++
		}
		all[i].limpo = final
	}

	fmt.Printf("=== limpeza de gêneros ===\n")
	fmt.Printf("  livros com gênero:        %d\n", len(all))
	fmt.Printf("  vocabulário antes:        %d rótulos distintos\n", len(antesDistintos))
	fmt.Printf("  vocabulário depois:       %d (frequência mínima: %d)\n", vocab.Size(), o.MinGenreFreq)
	fmt.Printf("  livros que mudam:         %d\n", mudam)
	fmt.Printf("  livros que ficam sem nenhum gênero: %d\n", esvaziam)
	fmt.Printf("\n  gêneros mantidos: %s\n", strings.Join(vocab.Top(40), " · "))

	if !o.Apply {
		fmt.Printf("\n  exemplos:\n")
		n := 0
		for _, b := range all {
			if sameSet(b.limpo, b.antes) || n >= 8 {
				continue
			}
			n++
			rel, _ := filepath.Rel(o.Library, b.path)
			fmt.Printf("    %s\n      antes:  %s\n      depois: %s\n",
				truncate(rel, 66), truncate(strings.Join(b.antes, ", "), 66),
				truncate(strings.Join(b.limpo, ", "), 66))
		}
		fmt.Printf("\nNADA FOI MODIFICADO (execução de inspeção). Use -apply para aplicar.\n")
		return nil
	}

	var ok, falhas atomic.Int64
	var wg2 sync.WaitGroup
	sem2 := make(chan struct{}, max(1, runtime.NumCPU()-2))
	start := time.Now()
	for _, b := range all {
		if sameSet(b.limpo, b.antes) {
			continue
		}
		wg2.Add(1)
		go func(b bookTags) {
			defer wg2.Done()
			sem2 <- struct{}{}
			defer func() { <-sem2 }()
			// Lista vazia apaga os gêneros: o ebook-meta aceita --tags "" para isso, e
			// nenhum gênero é melhor que gênero-ruído no filtro.
			if err := writeMeta(b.path, []string{"--tags", strings.Join(b.limpo, ",")}); err != nil {
				falhas.Add(1)
				return
			}
			ok.Add(1)
		}(b)
	}
	wg2.Wait()
	fmt.Printf("\naplicado em %s\n  livros atualizados: %d\n  falhas: %d\n",
		time.Since(start).Round(time.Second), ok.Load(), falhas.Load())
	return nil
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string{}, a...)
	y := append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func truncate(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// collect lista os livros que ainda precisam de trabalho. Filtrar aqui é o que torna o
// comando retomável e barato de repetir.
func collect(o Options) ([]item, error) {
	roots := o.Paths
	if len(roots) == 0 {
		roots = []string{o.Library}
	}
	var out []item
	seen := map[string]bool{}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil || d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".epub") || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			if seen[p] {
				return nil
			}
			seen[p] = true

			m, err := epub.ReadMeta(p)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(o.Library, p)
			base := filepath.Base(p)
			it := item{rel: rel, full: p, m: m,
				b: meta.Parse(strings.TrimSuffix(base, filepath.Ext(base)))}

			need := false
			if o.Enrich && (len(m.Tags) == 0 || m.ISBN == "" || m.Publisher == "" || !m.HasDesc) {
				need = true
			}
			if o.Relayout {
				pl := o.Target.Place(it.b, m)
				if filepath.Join(pl.Dir, pl.Filename) != rel || len(pl.MetaArgs) > 0 {
					need = true
				}
			}
			if !o.Enrich && !o.Relayout && len(o.Target.Place(it.b, m).MetaArgs) > 0 {
				need = true // só normalização de metadado local
			}
			if need {
				out = append(out, it)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].rel < out[j].rel })
	if o.Limit > 0 && o.Limit < len(out) {
		out = out[:o.Limit]
	}
	return out, nil
}

func preview(o Options, items []item) {
	n := len(items)
	if n > 12 {
		n = 12
	}
	for _, it := range items[:n] {
		fmt.Printf("  %s\n", it.rel)
		var faltam []string
		if len(it.m.Tags) == 0 {
			faltam = append(faltam, "gêneros")
		}
		if it.m.ISBN == "" {
			faltam = append(faltam, "ISBN")
		}
		if it.m.Publisher == "" {
			faltam = append(faltam, "editora")
		}
		if !it.m.HasDesc {
			faltam = append(faltam, "sinopse")
		}
		if len(faltam) > 0 && o.Enrich {
			fmt.Printf("      buscar: %s\n", strings.Join(faltam, ", "))
		}
		if args := o.Target.Place(it.b, it.m).MetaArgs; len(args) > 0 {
			fmt.Printf("      local:  %s\n", strings.Join(args, " "))
		}
		if o.Relayout {
			pl := o.Target.Place(it.b, it.m)
			if dst := filepath.Join(pl.Dir, pl.Filename); dst != it.rel {
				fmt.Printf("      mover:  -> %s\n", dst)
			}
		}
	}
	if len(items) > 12 {
		fmt.Printf("  ... e %d livro(s)\n", len(items)-12)
	}
	fmt.Printf("\nNADA FOI MODIFICADO (execução de inspeção). Use -apply para aplicar.\n")
}

func apply(o Options, items []item) error {
	var failLog *os.File
	if o.FailLog != "" {
		if f, err := os.Create(o.FailLog); err == nil {
			failLog = f
			defer f.Close()
			fmt.Fprintf(f, "arquivo\tmotivo\n")
		}
	}
	var logMu sync.Mutex

	var done, okMeta, okEnrich, moved, semDados, falhas atomic.Int64
	start := time.Now()

	// Progresso em goroutine própria: com horas de execução, saber o ritmo e o quanto
	// falta é o que permite decidir se vale esperar ou interromper.
	stop := make(chan struct{})
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				d := done.Load()
				if d == 0 {
					continue
				}
				el := time.Since(start)
				rate := float64(d) / el.Seconds()
				var eta string
				if rate > 0 {
					eta = (time.Duration(float64(int64(len(items))-d)/rate) * time.Second).Round(time.Second).String()
				}
				fmt.Printf("\r\033[K%d/%d · %d metadados · %d via API · %d falhas · %.2f/s · restam ~%s",
					d, len(items), okMeta.Load(), okEnrich.Load(), falhas.Load(), rate, eta)
			}
		}
	}()

	var wg sync.WaitGroup
	sem := make(chan struct{}, o.Workers)
	for _, it := range items {
		wg.Add(1)
		go func(it item) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			defer done.Add(1)

			var args []string
			args = append(args, o.Target.Place(it.b, it.m).MetaArgs...)

			if o.Enrich {
				title := it.m.Title
				if title == "" {
					title = it.b.Title
				}
				author := it.m.Author
				if author == "" {
					author = it.b.Author
				}
				r, err := enrich.FetchWithFallback(context.Background(), title, author,
					enrich.Options{Timeout: o.Timeout, TranslateTags: o.TagsPT})
				switch {
				case err != nil:
					falhas.Add(1)
					if failLog != nil {
						logMu.Lock()
						fmt.Fprintf(failLog, "%s\t%v\n", it.rel, err)
						logMu.Unlock()
					}
				case !r.Found:
					semDados.Add(1)
				default:
					ea := r.MetaArgs(len(it.m.Tags) > 0, it.m.ISBN != "", it.m.Publisher != "", it.m.HasDesc)
					if len(ea) > 0 {
						args = append(args, ea...)
						okEnrich.Add(1)
					} else {
						semDados.Add(1)
					}
				}
			}

			if len(args) > 0 {
				if err := writeMeta(it.full, args); err != nil {
					falhas.Add(1)
					if failLog != nil {
						logMu.Lock()
						fmt.Fprintf(failLog, "%s\tescrita: %v\n", it.rel, err)
						logMu.Unlock()
					}
					return
				}
				okMeta.Add(1)
			}

			if o.Relayout {
				m2, _ := epub.ReadMeta(it.full)
				pl := o.Target.Place(it.b, m2)
				dst := filepath.Join(o.Library, pl.Dir, pl.Filename)
				if dst != it.full {
					if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
						if _, err := os.Stat(dst); err != nil {
							if os.Rename(it.full, dst) == nil {
								moved.Add(1)
							}
						}
					}
				}
			}
		}(it)
	}
	wg.Wait()
	close(stop)

	fmt.Printf("\r\033[K")
	fmt.Printf("concluído em %s\n", time.Since(start).Round(time.Second))
	fmt.Printf("  metadados escritos:      %d\n", okMeta.Load())
	if o.Enrich {
		fmt.Printf("  completados via API:     %d\n", okEnrich.Load())
		fmt.Printf("  sem dados nas fontes:    %d\n", semDados.Load())
	}
	if o.Relayout {
		fmt.Printf("  arquivos movidos:        %d\n", moved.Load())
	}
	fmt.Printf("  falhas:                  %d\n", falhas.Load())
	if o.FailLog != "" && falhas.Load() > 0 {
		fmt.Printf("  log de falhas: %s\n", o.FailLog)
	}
	fmt.Printf("\nrode de novo a qualquer momento: quem já foi completado é pulado.\n")
	return nil
}

func writeMeta(path string, args []string) error {
	cmd := exec.Command(pythonBin, append([]string{metaBin, path}, args...)...)
	home, _ := os.UserHomeDir()
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + home, "LANG=C.UTF-8", "TMPDIR=" + os.TempDir()}
	if b, err := cmd.CombinedOutput(); err != nil {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		return fmt.Errorf("%s", lines[len(lines)-1])
	}
	return nil
}
