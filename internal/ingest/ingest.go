// Package ingest amarra as peças: indexar a biblioteca, comparar o que chegou e
// aplicar o layout do alvo.
package ingest

import (
	"context"
	"fmt"
	"io"
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
	"github.com/isaacmirandacampos/robooks/internal/index"
	"github.com/isaacmirandacampos/robooks/internal/meta"
	"github.com/isaacmirandacampos/robooks/internal/target"
)

const (
	pythonBin  = "/usr/bin/python3"
	convertBin = "/usr/bin/ebook-convert"
	metaBin    = "/usr/bin/ebook-meta"
)

// calibreEnv monta um ambiente mínimo para as ferramentas do calibre.
//
// O PATH explícito não é zelo: o shebang do ebook-convert é "#!/usr/bin/env python3" e
// gerenciadores de versão (mise, pyenv, conda) colocam um Python à frente que não tem
// os módulos do calibre, fazendo o script morrer com ModuleNotFoundError. Invocar
// /usr/bin/python3 diretamente e limpar o ambiente elimina a dependência do PATH de
// quem chama.
func calibreEnv() []string {
	home, _ := os.UserHomeDir()
	return []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + home,
		"LANG=C.UTF-8",
		"TMPDIR=" + os.TempDir(),
	}
}

func workersOrDefault(n int) int {
	if n > 0 {
		return n
	}
	if c := runtime.NumCPU() - 2; c > 0 {
		return c
	}
	return 1
}

// ---------- índice ----------

type IndexOptions struct {
	Library   string
	IndexPath string
	Workers   int
	Force     bool
}

type IndexResult struct {
	Total, Updated, Pruned int
}

// BuildIndex varre a biblioteca e atualiza as assinaturas, recalculando só o que mudou.
func BuildIndex(o IndexOptions) (IndexResult, error) {
	var res IndexResult
	ix, err := index.Load(o.IndexPath, o.Library)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aviso: %v\n", err)
	}
	if o.Force {
		ix = index.New(o.Library)
	}

	type job struct {
		rel  string
		size int64
		mod  int64
	}
	var jobs []job
	exists := map[string]bool{}

	err = filepath.WalkDir(o.Library, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return nil // uma pasta ilegível não deve abortar a varredura inteira
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(d.Name()), ".epub") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, rerr := filepath.Rel(o.Library, path)
		if rerr != nil {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		exists[rel] = true
		if ix.NeedsUpdate(rel, info.Size(), info.ModTime().Unix()) {
			jobs = append(jobs, job{rel, info.Size(), info.ModTime().Unix()})
		}
		return nil
	})
	if err != nil {
		return res, err
	}

	n := workersOrDefault(o.Workers)
	var wg sync.WaitGroup
	sem := make(chan struct{}, n)
	var mu sync.Mutex
	var done atomic.Int64

	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			full := filepath.Join(o.Library, j.rel)
			sig, words := epub.Signature(full)
			m, _ := epub.ReadMeta(full)
			e := &index.Entry{
				Path: j.rel, Size: j.size, ModTime: j.mod, Words: words, Sig: sig,
				Title: m.Title, Author: m.Author, Series: m.Series,
			}
			mu.Lock()
			ix.Put(e)
			mu.Unlock()
			if k := done.Add(1); k%500 == 0 {
				fmt.Printf("\r  indexando: %d/%d", k, len(jobs))
			}
		}(j)
	}
	wg.Wait()
	if len(jobs) > 0 {
		fmt.Printf("\r  indexando: %d/%d\n", len(jobs), len(jobs))
	}

	res.Pruned = ix.Prune(exists)
	res.Updated = len(jobs)
	res.Total = len(ix.Entries)
	return res, ix.Save(o.IndexPath)
}

// ---------- ingestão ----------

type Options struct {
	Inputs        []string
	Library       string
	IndexPath     string
	Target        target.Target
	Apply         bool
	Workers       int
	Similarity    float64
	OnDuplicate   string
	Quarantine    string
	Convert       bool
	Enrich        bool
	EnrichWorkers int
	TranslateTags bool
}

type outcome struct {
	src        string
	dest       string
	duplicate  *index.Match
	metaArgs   []string
	enrichArgs []string
	enrichNote string
	converted  bool
	err        error

	// Preenchidos quando há duplicata e o modo é "best": qual cópia vence e por quê.
	newWins bool
	why     string
}

// Run processa os arquivos de entrada.
func Run(o Options) error {
	ix, err := index.Load(o.IndexPath, o.Library)
	if err != nil {
		fmt.Fprintf(os.Stderr, "aviso: %v\n", err)
	}
	if len(ix.Entries) == 0 {
		fmt.Println("aviso: índice vazio — rode `robooks index` antes, senão nenhuma duplicata será detectada")
	}

	files, err := collectInputs(o.Inputs)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Println("nenhum arquivo de entrada encontrado (.epub, .mobi, .azw3)")
		return nil
	}

	fmt.Printf("entrada:    %d arquivo(s)\n", len(files))
	fmt.Printf("biblioteca: %s (%d livros no índice)\n", o.Library, len(ix.Entries))
	fmt.Printf("alvo:       %s — %s\n", o.Target.Name(), o.Target.Describe())
	fmt.Printf("duplicata:  semelhança >= %.0f%%, ação: %s\n\n", o.Similarity*100, o.OnDuplicate)

	results := make([]outcome, len(files))
	n := workersOrDefault(o.Workers)
	var wg sync.WaitGroup
	sem := make(chan struct{}, n)

	for i, f := range files {
		wg.Add(1)
		go func(i int, f string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = process(o, ix, f)
		}(i, f)
	}
	wg.Wait()

	return report(o, ix, results)
}

// process avalia um arquivo: converte se necessário, mede a assinatura, procura
// duplicata e calcula o destino. Não escreve nada na biblioteca.
func process(o Options, ix *index.Index, src string) outcome {
	res := outcome{src: src}

	work := src
	ext := strings.ToLower(filepath.Ext(src))
	if ext != ".epub" {
		if !o.Convert {
			res.err = fmt.Errorf("formato %s exige conversão (use -convert)", ext)
			return res
		}
		if !o.Apply {
			// Em inspeção não vale gastar uma conversão; o resto do plano depende do
			// epub convertido, então só se anuncia o que aconteceria.
			res.converted = true
			return res
		}
		out, err := convertToEpub(src)
		if err != nil {
			res.err = err
			return res
		}
		work = out
		res.converted = true
	}

	sig, _ := epub.Signature(work)
	if len(sig) == 0 {
		res.err = fmt.Errorf("não foi possível ler o texto do arquivo")
		return res
	}
	if matches := ix.FindSimilar(sig, o.Similarity, epub.Jaccard); len(matches) > 0 {
		res.duplicate = &matches[0]
		if o.OnDuplicate == DupBest {
			newer := candidateFor(work)
			existing := candidateFromIndex(o.Library, matches[0].Entry)
			res.newWins = betterCopy(newer, existing)
			res.why = describeChoice(newer, existing)
			if res.newWins {
				// O novo substitui: precisa do destino calculado, e é o caminho do que
				// ele substitui, para não criar uma segunda cópia noutro lugar.
				res.dest = matches[0].Entry.Path
				m, _ := epub.ReadMeta(work)
				b := meta.Parse(strings.TrimSuffix(filepath.Base(work), filepath.Ext(work)))
				res.metaArgs = o.Target.Place(b, m).MetaArgs
			}
		}
		return res
	}

	m, _ := epub.ReadMeta(work)
	b := meta.Parse(strings.TrimSuffix(filepath.Base(work), filepath.Ext(work)))
	pl := o.Target.Place(b, m)
	res.dest = filepath.Join(pl.Dir, pl.Filename)
	res.metaArgs = pl.MetaArgs
	if o.Enrich {
		res.enrichArgs, res.enrichNote = enrichOne(o, m, b)
	}
	return res
}

// enrichOne consulta fontes externas para completar o que falta no arquivo. Só é
// chamado quando -enrich está ligado, porque cada consulta custa entre 7 e 25 segundos.
func enrichOne(o Options, m epub.Meta, b meta.Book) ([]string, string) {
	title := m.Title
	if title == "" {
		title = b.Title
	}
	author := m.Author
	if author == "" {
		author = b.Author
	}
	if title == "" {
		return nil, "sem título para consultar"
	}

	faltam := len(m.Tags) == 0 || m.ISBN == "" || m.Publisher == "" || !m.HasDesc
	if !faltam {
		return nil, "nada a completar"
	}

	r, err := enrich.FetchWithFallback(context.Background(), title, author, enrich.Options{
		Timeout:       90 * time.Second,
		TranslateTags: o.TranslateTags,
	})
	if err != nil {
		return nil, "consulta externa: " + err.Error()
	}
	if !r.Found {
		return nil, "não encontrado nas fontes externas"
	}
	args := r.MetaArgs(len(m.Tags) > 0, m.ISBN != "", m.Publisher != "", m.HasDesc)
	if len(args) == 0 {
		return nil, "fontes não trouxeram nada novo"
	}
	var campos []string
	for i := 0; i < len(args); i += 2 {
		campos = append(campos, strings.TrimPrefix(args[i], "--"))
	}
	return args, "completa: " + strings.Join(campos, ", ")
}

func report(o Options, ix *index.Index, results []outcome) error {
	var novos, dups, subs, erros int
	for _, r := range results {
		switch {
		case r.err != nil:
			erros++
		case r.duplicate != nil && r.newWins:
			subs++
		case r.duplicate != nil:
			dups++
		default:
			novos++
		}
	}

	for _, r := range results {
		name := filepath.Base(r.src)
		switch {
		case r.err != nil:
			fmt.Printf("  ERRO       %s: %v\n", name, r.err)
		case r.duplicate != nil && r.newWins:
			fmt.Printf("  SUBSTITUI  %s\n             %.0f%% igual a %s\n             %s\n",
				name, r.duplicate.Similarity*100, r.duplicate.Entry.Path, r.why)
		case r.duplicate != nil:
			fmt.Printf("  DUPLICATA  %s\n             %.0f%% igual a %s\n",
				name, r.duplicate.Similarity*100, r.duplicate.Entry.Path)
			if r.why != "" {
				fmt.Printf("             %s\n", r.why)
			}
		default:
			fmt.Printf("  NOVO       %s\n             -> %s\n", name, r.dest)
			if len(r.metaArgs) > 0 {
				fmt.Printf("             metadados: %s\n", strings.Join(r.metaArgs, " "))
			}
			if r.enrichNote != "" {
				fmt.Printf("             externo: %s\n", r.enrichNote)
			}
		}
	}

	fmt.Printf("\nresumo: %d novo(s), %d substitui\u00e7\u00e3o(\u00f5es), %d duplicata(s) descartada(s), %d erro(s)\n",
		novos, subs, dups, erros)
	if !o.Apply {
		fmt.Println("\nNADA FOI MODIFICADO (execução de inspeção). Use -apply para aplicar.")
		return nil
	}
	return apply(o, ix, results)
}

func apply(o Options, ix *index.Index, results []outcome) error {
	quarantine := o.Quarantine
	if quarantine == "" && len(o.Inputs) > 0 {
		quarantine = filepath.Join(firstDir(o.Inputs[0]), "_duplicatas")
	}

	var moved, replaced, skipped, failed int
	for _, r := range results {
		if r.err != nil {
			failed++
			continue
		}
		if r.duplicate != nil {
			if o.OnDuplicate == DupBest && r.newWins {
				// Substituição no lugar: escreve os metadados, troca o arquivo e
				// atualiza o índice. O antigo vai para a quarentena em vez de sumir —
				// a decisão é heurística e precisa ser reversível.
				dst := filepath.Join(o.Library, r.duplicate.Entry.Path)
				if len(r.metaArgs) > 0 {
					if err := writeMeta(r.src, r.metaArgs); err != nil {
						fmt.Printf("aviso: metadados não aplicados em %s: %v\n", filepath.Base(r.src), err)
					}
				}
				if err := os.MkdirAll(quarantine, 0o755); err == nil {
					_ = moveFile(dst, uniquePath(filepath.Join(quarantine, filepath.Base(dst))))
				}
				if err := moveFile(r.src, dst); err != nil {
					fmt.Printf("FALHA substituir %s: %v\n", r.duplicate.Entry.Path, err)
					failed++
					continue
				}
				replaced++
				if sig, words := epub.Signature(dst); len(sig) > 0 {
					if st, serr := os.Stat(dst); serr == nil {
						m, _ := epub.ReadMeta(dst)
						ix.Put(&index.Entry{
							Path: r.duplicate.Entry.Path, Size: st.Size(), ModTime: st.ModTime().Unix(),
							Words: words, Sig: sig, Title: m.Title, Author: m.Author, Series: m.Series,
						})
					}
				}
				continue
			}
			switch o.OnDuplicate {
			case DupBest, DupQuarantine:
				if err := os.MkdirAll(quarantine, 0o755); err == nil {
					if err := moveFile(r.src, filepath.Join(quarantine, filepath.Base(r.src))); err != nil {
						fmt.Printf("FALHA mover duplicata %s: %v\n", filepath.Base(r.src), err)
						failed++
						continue
					}
				}
			case DupReplace:
				dst := filepath.Join(o.Library, r.duplicate.Entry.Path)
				if err := os.MkdirAll(quarantine, 0o755); err == nil {
					_ = moveFile(dst, uniquePath(filepath.Join(quarantine, filepath.Base(dst))))
				}
				if err := moveFile(r.src, dst); err != nil {
					fmt.Printf("FALHA substituir %s: %v\n", r.duplicate.Entry.Path, err)
					failed++
					continue
				}
				replaced++
				continue
			}
			skipped++
			continue
		}

		dst := filepath.Join(o.Library, r.dest)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			fmt.Printf("FALHA criar pasta para %s: %v\n", filepath.Base(r.src), err)
			failed++
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			fmt.Printf("  PULADO (destino já existe): %s\n", r.dest)
			skipped++
			continue
		}
		// Escreve os metadados antes de mover: se algo falhar, a biblioteca não recebe
		// arquivo pela metade.
		allArgs := append(append([]string{}, r.metaArgs...), r.enrichArgs...)
		if len(allArgs) > 0 {
			if err := writeMeta(r.src, allArgs); err != nil {
				fmt.Printf("aviso: metadados não aplicados em %s: %v\n", filepath.Base(r.src), err)
			}
		}
		if err := moveFile(r.src, dst); err != nil {
			fmt.Printf("FALHA mover %s: %v\n", filepath.Base(r.src), err)
			failed++
			continue
		}
		moved++

		// Mantém o índice em dia para que dois arquivos iguais na mesma execução não
		// entrem os dois.
		if sig, words := epub.Signature(dst); len(sig) > 0 {
			if st, err := os.Stat(dst); err == nil {
				m, _ := epub.ReadMeta(dst)
				ix.Put(&index.Entry{
					Path: r.dest, Size: st.Size(), ModTime: st.ModTime().Unix(),
					Words: words, Sig: sig, Title: m.Title, Author: m.Author, Series: m.Series,
				})
			}
		}
	}

	fmt.Printf("\naplicado:\n  adicionados:  %d\n  substituídos: %d\n  ignorados:    %d\n  falhas:       %d\n",
		moved, replaced, skipped, failed)
	if err := ix.Save(o.IndexPath); err != nil {
		return fmt.Errorf("índice não salvo: %w", err)
	}
	fmt.Printf("  índice atualizado: %s\n", o.IndexPath)
	return nil
}

// ---------- verificação avulsa ----------

// Check responde se os arquivos já existem na biblioteca. Devolve true se algum for
// duplicata, para permitir uso em script pelo código de saída.
func Check(paths []string, lib, idxPath string, threshold float64) (bool, error) {
	ix, err := index.Load(idxPath, lib)
	if err != nil {
		return false, err
	}
	any := false
	for _, p := range paths {
		sig, _ := epub.Signature(p)
		if len(sig) == 0 {
			fmt.Printf("  ?  %s: não foi possível ler o texto\n", filepath.Base(p))
			continue
		}
		matches := ix.FindSimilar(sig, threshold, epub.Jaccard)
		if len(matches) == 0 {
			fmt.Printf("  NOVO  %s\n", filepath.Base(p))
			continue
		}
		any = true
		fmt.Printf("  JÁ EXISTE  %s\n", filepath.Base(p))
		for i, m := range matches {
			if i >= 3 {
				break
			}
			fmt.Printf("      %.0f%%  %s\n", m.Similarity*100, m.Entry.Path)
		}
	}
	return any, nil
}

// ---------- utilitários ----------

func collectInputs(inputs []string) ([]string, error) {
	var out []string
	for _, in := range inputs {
		st, err := os.Stat(in)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if isBook(in) {
				out = append(out, in)
			}
			continue
		}
		err = filepath.WalkDir(in, func(p string, d fs.DirEntry, werr error) error {
			if werr != nil || d.IsDir() || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			// A pasta de quarentena é saída do próprio comando; reprocessá-la criaria
			// um laço.
			if strings.Contains(p, string(os.PathSeparator)+"_duplicatas"+string(os.PathSeparator)) {
				return nil
			}
			if isBook(p) {
				out = append(out, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func isBook(p string) bool {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".epub", ".mobi", ".azw3", ".azw":
		return true
	}
	return false
}

func firstDir(p string) string {
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return p
	}
	return filepath.Dir(p)
}

// convertToEpub roda o calibre com --flow-size 0.
//
// A opção desliga a divisão por tamanho do HTML. Sem ela, livros cujo conteúdo não tem
// ponto de quebra abaixo do limite abortam com "Could not find reasonable point at
// which to split" e não geram saída nenhuma. Não altera texto nem imagens: só deixa os
// arquivos internos maiores.
func convertToEpub(src string) (string, error) {
	out := strings.TrimSuffix(src, filepath.Ext(src)) + ".epub"
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	cmd := exec.Command(pythonBin, convertBin, src, out, "--flow-size", "0")
	cmd.Env = calibreEnv()
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("conversão falhou: %s", lastLine(string(b)))
	}
	return out, nil
}

func writeMeta(path string, args []string) error {
	cmd := exec.Command(pythonBin, append([]string{metaBin, path}, args...)...)
	cmd.Env = calibreEnv()
	if b, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s", lastLine(string(b)))
	}
	return nil
}

// moveFile move entre diretórios, caindo para cópia quando origem e destino estão em
// filesystems diferentes (o caso comum: download num disco, biblioteca noutro).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	in.Close()
	return os.Remove(src)
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}

// uniquePath evita sobrescrever na quarentena quando o mesmo nome chega duas vezes.
func uniquePath(p string) string {
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for n := 2; n < 1000; n++ {
		cand := fmt.Sprintf("%s (%d)%s", base, n, ext)
		if _, err := os.Stat(cand); err != nil {
			return cand
		}
	}
	return p
}
