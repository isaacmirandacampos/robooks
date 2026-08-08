package edit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/isaacmirandacampos/robooks/internal/epub"
)

// ImportTags grava nos epubs os gêneros que hoje só existem no catálogo do servidor.
//
// O caso: o servidor enriquece metadados por conta própria e guarda o resultado no banco
// dele, sem tocar nos arquivos. Isso deixa o acervo com duas verdades — a do banco, boa,
// e a dos arquivos, vazia — e a do banco é a frágil: basta um rescan lendo os arquivos
// para o trabalho sumir. Escrever de volta nos epubs inverte a dependência, e aí o
// arquivo passa a ser a fonte que sobrevive a qualquer reinstalação do servidor.
//
// A entrada é um TSV de "caminho<TAB>gênero, gênero, gênero", pela mesma razão dos outros
// comandos: quem sabe montar o caminho é o servidor, e o robooks não fala com nenhum.
type ImportOptions struct {
	File    string // TSV com caminho e gêneros
	Library string
	Apply   bool
	Workers int
	// Merge acrescenta aos gêneros que o arquivo já tem, em vez de substituí-los.
	Merge bool
}

type tagRow struct {
	path  string
	tags  []string
	atual []string
}

func ImportTags(o ImportOptions) error {
	if o.Workers <= 0 {
		o.Workers = max(1, runtime.NumCPU()-2)
	}

	linhas, err := lerTagsTSV(o.File)
	if err != nil {
		return err
	}
	fmt.Printf("%d livros na entrada\n", len(linhas))

	// Lê o estado atual de cada arquivo antes de decidir. Sem isso não dá para saber
	// quem já está correto, e um comando que reescreve 11 mil epubs sem necessidade
	// custa horas e ainda invalida o índice de conteúdo inteiro.
	var (
		mu        sync.Mutex
		pendentes []tagRow
		ausentes  int
		iguais    int
	)
	var wg sync.WaitGroup
	sem := make(chan struct{}, o.Workers)
	for _, r := range linhas {
		wg.Add(1)
		go func(r tagRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := os.Stat(r.path); err != nil {
				mu.Lock()
				ausentes++
				mu.Unlock()
				return
			}
			m, err := epub.ReadMeta(r.path)
			if err == nil {
				r.atual = m.Tags
			}
			alvo := r.tags
			if o.Merge {
				alvo = uniaoOrdenada(r.atual, r.tags)
			}
			if sameSet(alvo, r.atual) {
				mu.Lock()
				iguais++
				mu.Unlock()
				return
			}
			r.tags = alvo
			mu.Lock()
			pendentes = append(pendentes, r)
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	fmt.Printf("  já corretos:      %d\n", iguais)
	fmt.Printf("  arquivo ausente:  %d\n", ausentes)
	fmt.Printf("  a atualizar:      %d\n", len(pendentes))

	if len(pendentes) == 0 {
		fmt.Println("\nnada a fazer.")
		return nil
	}
	sort.Slice(pendentes, func(i, j int) bool { return pendentes[i].path < pendentes[j].path })

	if !o.Apply {
		fmt.Printf("\n  exemplos:\n")
		for i, r := range pendentes {
			if i >= 8 {
				break
			}
			rel, _ := filepath.Rel(o.Library, r.path)
			fmt.Printf("    %s\n      antes:  %s\n      depois: %s\n",
				truncate(rel, 66), truncate(strings.Join(r.atual, ", "), 66),
				truncate(strings.Join(r.tags, ", "), 66))
		}
		fmt.Printf("\nNADA FOI MODIFICADO (execução de inspeção). Use -apply para aplicar.\n")
		return nil
	}

	var ok, falhas atomic.Int64
	var wg2 sync.WaitGroup
	sem2 := make(chan struct{}, o.Workers)
	start := time.Now()
	for _, r := range pendentes {
		wg2.Add(1)
		go func(r tagRow) {
			defer wg2.Done()
			sem2 <- struct{}{}
			defer func() { <-sem2 }()
			if err := writeMeta(r.path, []string{"--tags", strings.Join(r.tags, ",")}); err != nil {
				falhas.Add(1)
				return
			}
			ok.Add(1)
		}(r)
	}
	wg2.Wait()

	fmt.Printf("\naplicado em %s\n  livros atualizados: %d\n  falhas: %d\n",
		time.Since(start).Round(time.Second), ok.Load(), falhas.Load())
	if ok.Load() > 0 {
		fmt.Println("\nos arquivos mudaram: rode 'robooks index' para atualizar o índice de conteúdo.")
	}
	return nil
}

func lerTagsTSV(path string) ([]tagRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrindo %s: %w", path, err)
	}
	defer f.Close()

	var out []tagRow
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		linha := sc.Text()
		if strings.TrimSpace(linha) == "" {
			continue
		}
		campos := strings.SplitN(linha, "\t", 2)
		if len(campos) < 2 {
			continue
		}
		p := strings.TrimSpace(campos[0])
		if p == "" {
			continue
		}
		var tags []string
		vistos := map[string]bool{}
		for _, t := range strings.Split(campos[1], ",") {
			t = strings.TrimSpace(t)
			if t == "" || vistos[strings.ToLower(t)] {
				continue
			}
			vistos[strings.ToLower(t)] = true
			tags = append(tags, t)
		}
		if len(tags) == 0 {
			continue
		}
		sort.Strings(tags)
		out = append(out, tagRow{path: p, tags: tags})
	}
	return out, sc.Err()
}

func uniaoOrdenada(a, b []string) []string {
	vistos := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		s = strings.TrimSpace(s)
		if s == "" || vistos[strings.ToLower(s)] {
			continue
		}
		vistos[strings.ToLower(s)] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
