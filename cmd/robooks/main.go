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

	"github.com/isaacdmcampos/robooks/internal/index"
	"github.com/isaacdmcampos/robooks/internal/ingest"
	"github.com/isaacdmcampos/robooks/internal/target"
)

const usage = `robooks — prepara ebooks para uma biblioteca organizada

USO
  robooks <comando> [flags] [caminho]

COMANDOS
  index     varre a biblioteca e atualiza o índice de conteúdo
  ingest    processa arquivos novos: dedup, metadados e layout do alvo
  check     verifica se um arquivo já existe na biblioteca
  targets   lista os alvos suportados

Sem -apply, todo comando apenas relata o que faria.
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

func defaultLibrary() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "kavita", "data", "Livros")
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
	fmt.Printf("  índice: %s (%s)\n", p, humanSize(fileSize(p)))
	fmt.Printf("  tempo: %s\n", time.Since(start).Round(time.Millisecond))
	return 0
}

func cmdIngest(args []string) int {
	fs := flag.NewFlagSet("ingest", flag.ExitOnError)
	lib := fs.String("lib", defaultLibrary(), "raiz da biblioteca de destino")
	tgt := fs.String("target", "kavita", "alvo: "+strings.Join(target.Names(), ", "))
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
	fs.Parse(args)

	if fs.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "informe a pasta ou arquivo de entrada\n\nex: robooks ingest ~/Downloads/livros")
		return 2
	}
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

func fileSize(p string) int64 {
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
