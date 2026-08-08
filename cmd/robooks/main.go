// robooks prepara ebooks baixados para uma biblioteca já organizada, sem criar
// duplicata e sem depender do servidor de destino.
//
// O fluxo é sempre o mesmo: indexar o que já existe, comparar o que chegou contra esse
// índice e só então mover para o layout que o alvo espera. Nada é escrito sem -apply.
//
//	robooks index                      constrói/atualiza o índice da biblioteca
//	robooks ingest ~/Downloads/livros  mostra o que aconteceria com os arquivos novos
//	robooks ingest -apply ~/Downloads/livros
//	robooks check arquivo.epub         só pergunta "isto já existe na biblioteca?"
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/isaacmirandacampos/robooks/internal/edit"
	"github.com/isaacmirandacampos/robooks/internal/index"
	"github.com/isaacmirandacampos/robooks/internal/ingest"
	"github.com/isaacmirandacampos/robooks/internal/target"
)

const usage = `robooks — prepara ebooks para uma biblioteca organizada

USO
  robooks <comando> [flags] [caminho]

COMANDOS
  index     varre a biblioteca e atualiza o índice de conteúdo
  ingest    processa arquivos novos: dedup, metadados e layout do alvo
  check     verifica se um arquivo já existe na biblioteca
  edit      completa metadados e reaplica o layout nos livros JÁ na biblioteca
  authors   acha autores duplicados numa lista do catálogo (lê TSV do stdin)
  targets   lista os alvos suportados

EXEMPLOS
  robooks index                                  indexa a biblioteca
  robooks ingest ~/Downloads/*.epub              inspeção, alvo kavita (padrão)
  robooks ingest -target calibre ~/Downloads     inspeção, alvo calibre
  robooks ingest -enrich -apply ~/Downloads      aplica, buscando gêneros/ISBN
  robooks edit -enrich                           inspeciona o que falta na biblioteca
  robooks edit -enrich -apply                    completa gêneros/ISBN de todo o acervo
  ... | robooks authors                          relatório de autores duplicados
  ... | robooks authors -safe -sql               SQL de merge, só os casos seguros

As flags vêm SEMPRE antes dos caminhos. Sem -apply, tudo é apenas inspeção.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "index":
		os.Exit(cmdIndex(args))
	case "ingest":
		os.Exit(cmdIngest(args))
	case "check":
		os.Exit(cmdCheck(args))
	case "edit":
		os.Exit(cmdEdit(args))
	case "genres":
		os.Exit(cmdGenres(args))
	case "authors":
		os.Exit(cmdAuthors(args))
	case "targets":
		os.Exit(cmdTargets())
	case "-h", "--help", "help":
		fmt.Print(usage)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "comando desconhecido %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
}

// checkStrayFlags aborta quando aparece uma flag depois dos caminhos.
//
// O pacote flag para de interpretar no primeiro argumento posicional, então
// "robooks ingest downloads/* -target calibre" rodaria alegremente no alvo padrão e
// ainda trataria "-target" como se fosse um arquivo. Falhar aqui é melhor que aplicar
// no alvo errado sem avisar.
func checkStrayFlags(args []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			fmt.Fprintf(os.Stderr,
				"erro: a flag %q veio depois dos caminhos e seria ignorada.\n"+
					"      as flags precisam vir antes:  robooks ingest %s <caminhos>\n", a, a)
			os.Exit(2)
		}
	}
}

func defaultLibrary() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Documents", "Livros")
}

func cmdTargets() int {
	names := target.Names()
	sort.Strings(names)
	fmt.Println("alvos suportados:")
	for _, n := range names {
		t, _ := target.Get(n)
		fmt.Printf("  %-10s %s\n", n, t.Describe())
	}
	return 0
}

func cmdIndex(args []string) int {
	fs := flag.NewFlagSet("index", flag.ExitOnError)
	lib := fs.String("lib", defaultLibrary(), "raiz da biblioteca")
	idxPath := fs.String("index", "", "arquivo de índice (padrão: ~/.cache/robooks/index.gz)")
	workers := fs.Int("workers", 0, "paralelismo (padrão NumCPU-2)")
	force := fs.Bool("force", false, "recalcular tudo, ignorando o índice atual")
	fs.Parse(args)

	p := *idxPath
	if p == "" {
		p = index.DefaultPath(*lib)
	}
	start := time.Now()
	res, err := ingest.BuildIndex(ingest.IndexOptions{
		Library: *lib, IndexPath: p, Workers: *workers, Force: *force,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 1
	}
	fmt.Printf("\nbiblioteca: %s\n", *lib)
	fmt.Printf("  livros indexados: %d\n", res.Total)
	fmt.Printf("  novos/atualizados: %d\n", res.Updated)
	fmt.Printf("  removidos do índice: %d\n", res.Pruned)
	fmt.Printf("  índice: %s (%s)\n", p, humanSize(fileSizeOf(p)))
	fmt.Printf("  tempo: %s\n", time.Since(start).Round(time.Millisecond))
	return 0
}

func cmdIngest(args []string) int {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	lib := fs.String("lib", defaultLibrary(), "raiz da biblioteca de destino")
	tgt := fs.String("target", "grimmory", "alvo: "+strings.Join(target.Names(), ", "))
	idxPath := fs.String("index", "", "arquivo de índice")
	apply := fs.Bool("apply", false, "aplicar as mudanças (sem isso, apenas relata)")
	workers := fs.Int("workers", 0, "paralelismo (padrão NumCPU-2)")
	threshold := fs.Float64("similarity", 0.85, "limiar de semelhança de conteúdo para considerar duplicata")
	onDup := fs.String("on-duplicate", ingest.DupBest,
		"duplicata: best (mantém a melhor cópia), skip, quarantine, replace")
	quarantine := fs.String("quarantine", "", "pasta para duplicatas (padrão: <entrada>/_duplicatas)")
	convert := fs.Bool("convert", true, "converter .mobi/.azw3 para .epub via calibre")
	enrichFlag := fs.Bool("enrich", false, "consultar fontes externas para completar gêneros, ISBN, editora e sinopse (7-25s por livro)")
	ptTags := fs.Bool("tags-pt", true, "com -enrich, traduzir os gêneros para português")
	minFreq := fs.Int("min-genre-freq", 3,
		"gênero só entra se a biblioteca já o usa N vezes ou se for canônico (0 desliga o filtro)")
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "informe a pasta ou arquivo de entrada\n\nex: robooks ingest ~/Downloads/livros")
		return 2
	}
	checkStrayFlags(fs.Args())
	t, err := target.Get(*tgt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 2
	}
	p := *idxPath
	if p == "" {
		p = index.DefaultPath(*lib)
	}
	err = ingest.Run(ingest.Options{
		Inputs:        fs.Args(),
		Library:       *lib,
		IndexPath:     p,
		Target:        t,
		Apply:         *apply,
		Workers:       *workers,
		Similarity:    *threshold,
		OnDuplicate:   *onDup,
		Quarantine:    *quarantine,
		Convert:       *convert,
		Enrich:        *enrichFlag,
		TranslateTags: *ptTags,
		MinGenreFreq:  *minFreq,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 1
	}
	return 0
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	lib := fs.String("lib", defaultLibrary(), "raiz da biblioteca")
	idxPath := fs.String("index", "", "arquivo de índice")
	threshold := fs.Float64("similarity", 0.85, "limiar de semelhança")
	fs.Parse(args)
	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "informe um arquivo .epub")
		return 2
	}
	checkStrayFlags(fs.Args())
	p := *idxPath
	if p == "" {
		p = index.DefaultPath(*lib)
	}
	dup, err := ingest.Check(fs.Args(), *lib, p, *threshold)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 1
	}
	if dup {
		return 1 // permite usar em script: exit 1 = já existe
	}
	return 0
}

func cmdEdit(args []string) int {
	fs := flag.NewFlagSet("edit", flag.ExitOnError)
	lib := fs.String("lib", defaultLibrary(), "raiz da biblioteca")
	tgt := fs.String("target", "grimmory", "alvo cujas convenções aplicar: "+strings.Join(target.Names(), ", "))
	apply := fs.Bool("apply", false, "aplicar as mudanças (sem isso, apenas relata)")
	enrichFlag := fs.Bool("enrich", false, "consultar fontes externas para completar gêneros, ISBN, editora e sinopse")
	relayout := fs.Bool("relayout", false, "mover os arquivos para o layout do alvo")
	workers := fs.Int("workers", 0, "paralelismo (padrão: 4 com -enrich, NumCPU-2 sem)")
	limit := fs.Int("limit", 0, "processar no máximo N livros (útil para testar antes de rodar tudo)")
	timeout := fs.Duration("timeout", 90*time.Second, "tempo máximo por consulta externa")
	ptTags := fs.Bool("tags-pt", true, "traduzir os gêneros para português")
	failLog := fs.String("faillog", "robooks-falhas.log", "arquivo de log das falhas")
	cleanGenres := fs.Bool("clean-genres", false, "limpar e unificar os gêneros existentes (remove ruído, traduz, corta a cauda longa)")
	minFreq := fs.Int("min-genre-freq", 3, "com -clean-genres, frequência mínima para um gênero valer um item no filtro")
	detectSeries := fs.Bool("detect-series", false, "descobrir séries não declaradas a partir dos títulos do mesmo autor")
	minMembers := fs.Int("min-series-volumes", 3, "com -detect-series, volumes mínimos para aceitar uma série")
	excludeSeries := fs.String("exclude-series", "", "nomes de série a vetar, separados por vírgula (a detecção é heurística)")
	seriesLog := fs.String("serieslog", "robooks-series.tsv", "log reversível das séries gravadas")
	importTags := fs.String("import-tags", "", "TSV \"caminho<TAB>gêneros\" para gravar nos epubs (ex: exportado do catálogo)")
	mergeTags := fs.Bool("merge-tags", false, "com -import-tags, somar aos gêneros do arquivo em vez de substituir")
	verbose := fs.Bool("v", false, "com -import-tags, listar todas as divergências em vez de oito exemplos")
	fs.Parse(args)
	checkStrayFlags(fs.Args())

	t, err := target.Get(*tgt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 2
	}
	// A importação de tags é um modo próprio: a origem dos gêneros é um arquivo, não a
	// biblioteca nem uma consulta externa, então nada do pipeline do Run se aplica.
	if *importTags != "" {
		if err := edit.ImportTags(edit.ImportOptions{
			File: *importTags, Library: *lib, Apply: *apply,
			Workers: *workers, Merge: *mergeTags, Verbose: *verbose,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			return 1
		}
		return 0
	}

	if err := edit.Run(edit.Options{
		Library: *lib, Paths: fs.Args(), Target: t, Apply: *apply,
		Enrich: *enrichFlag, Relayout: *relayout, Workers: *workers,
		Limit: *limit, Timeout: *timeout, TagsPT: *ptTags, FailLog: *failLog,
		CleanGenres: *cleanGenres, MinGenreFreq: *minFreq,
		DetectSeries: *detectSeries, MinSeriesMembers: *minMembers,
		ExcludeSeries: strings.Split(*excludeSeries, ","), SeriesLog: *seriesLog,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		return 1
	}
	return 0
}

func fileSizeOf(p string) int64 {
	st, err := os.Stat(p)
	if err != nil {
		return 0
	}
	return st.Size()
}

func humanSize(n int64) string {
	switch {
	case n > 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n > 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}
