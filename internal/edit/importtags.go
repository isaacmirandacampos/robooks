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
	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// ImportTags grava nos epubs os metadados que hoje só existem no catálogo do servidor.
//
// O caso: o servidor enriquece metadados por conta própria e guarda o resultado no banco
// dele, sem tocar nos arquivos. Isso deixa o acervo com duas verdades — a do banco, boa,
// e a dos arquivos, desatualizada — e a do banco é a frágil: basta um rescan lendo os
// arquivos para o trabalho sumir. Escrever de volta nos epubs inverte a dependência, e aí
// o arquivo passa a ser a fonte que sobrevive a qualquer reinstalação do servidor.
//
// A entrada é um TSV, pela mesma razão dos outros comandos: quem sabe montar o caminho é
// o servidor, e o robooks não fala com nenhum. Duas formas são aceitas:
//
//	caminho<TAB>gênero, gênero            (sem cabeçalho — só gêneros)
//	path<TAB>authors<TAB>tags             (com cabeçalho — colunas nomeadas)
//
// A primeira existe porque veio antes; a segunda porque autor tem o mesmo problema que
// gênero e não faria sentido um comando separado para cada campo.
type ImportOptions struct {
	File    string
	Library string
	Apply   bool
	Workers int
	// Merge acrescenta aos valores que o arquivo já tem, em vez de substituí-los.
	// Só se aplica a gêneros: autor não é lista acumulável, é identificação.
	Merge bool
	// Verbose lista todas as divergências em vez de oito exemplos. Numa inspeção o
	// que importa é conferir o conjunto inteiro antes de escrever.
	Verbose bool
}

// linha é o que o TSV pede para um arquivo. Campo vazio significa "não mexa neste",
// e é o que separa "o catálogo não sabe" de "o catálogo diz que é vazio".
type linha struct {
	path    string
	tags    []string
	authors []string
	// estado lido do arquivo, preenchido durante a análise
	tagsAtuais    []string
	authorsAtuais []string
}

// muda diz o que precisa ser reescrito neste arquivo.
type mudanca struct {
	tags, authors bool
}

func ImportTags(o ImportOptions) error {
	if o.Workers <= 0 {
		o.Workers = max(1, runtime.NumCPU()-2)
	}

	linhas, temAutores, err := lerImportTSV(o.File)
	if err != nil {
		return err
	}
	campos := "gêneros"
	if temAutores {
		campos = "gêneros e autores"
	}
	fmt.Printf("%d livros na entrada (%s)\n", len(linhas), campos)

	// Lê o estado atual antes de decidir. Sem isso não dá para saber quem já está
	// correto, e um comando que reescreve onze mil epubs sem necessidade custa horas e
	// ainda invalida o índice de conteúdo inteiro.
	var (
		mu        sync.Mutex
		pendentes []linha
		mudancas  []mudanca
		ausentes  int
		iguais    int
		soTags    int
		soAutores int
	)
	var wg sync.WaitGroup
	sem := make(chan struct{}, o.Workers)
	for _, r := range linhas {
		wg.Add(1)
		go func(r linha) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := os.Stat(r.path); err != nil {
				mu.Lock()
				ausentes++
				mu.Unlock()
				return
			}
			if m, err := epub.ReadMeta(r.path); err == nil {
				r.tagsAtuais = m.Tags
				r.authorsAtuais = m.Authors
			}

			var mud mudanca
			if len(r.tags) > 0 {
				alvo := r.tags
				if o.Merge {
					alvo = uniaoOrdenada(r.tagsAtuais, r.tags)
				}
				if !sameSet(alvo, r.tagsAtuais) {
					r.tags, mud.tags = alvo, true
				}
			}
			// Autor não entra em união: dois nomes para a mesma pessoa é justamente o
			// problema que o merge no catálogo resolveu, e somar traria o duplicado de
			// volta para dentro do arquivo.
			if len(r.authors) > 0 && !sameSet(r.authors, r.authorsAtuais) {
				mud.authors = true
			}

			mu.Lock()
			defer mu.Unlock()
			switch {
			case !mud.tags && !mud.authors:
				iguais++
			default:
				if mud.tags && !mud.authors {
					soTags++
				}
				if mud.authors && !mud.tags {
					soAutores++
				}
				pendentes = append(pendentes, r)
				mudancas = append(mudancas, mud)
			}
		}(r)
	}
	wg.Wait()

	fmt.Printf("  já corretos:      %d\n", iguais)
	fmt.Printf("  arquivo ausente:  %d\n", ausentes)
	fmt.Printf("  a atualizar:      %d", len(pendentes))
	if temAutores {
		fmt.Printf("  (só gêneros: %d, só autores: %d, ambos: %d)",
			soTags, soAutores, len(pendentes)-soTags-soAutores)
	}
	fmt.Println()

	if len(pendentes) == 0 {
		fmt.Println("\nnada a fazer.")
		return nil
	}

	ordem := make([]int, len(pendentes))
	for i := range ordem {
		ordem[i] = i
	}
	sort.Slice(ordem, func(i, j int) bool { return pendentes[ordem[i]].path < pendentes[ordem[j]].path })

	if !o.Apply {
		fmt.Printf("\n  exemplos:\n")
		for n, idx := range ordem {
			if n >= 8 && !o.Verbose {
				fmt.Printf("    ... e mais %d (use -v para ver todas)\n", len(ordem)-n)
				break
			}
			r, mud := pendentes[idx], mudancas[idx]
			rel, _ := filepath.Rel(o.Library, r.path)
			fmt.Printf("    %s\n", truncate(rel, 66))
			if mud.authors {
				fmt.Printf("      autor  antes:  %s\n      autor  depois: %s\n",
					truncate(strings.Join(r.authorsAtuais, ", "), 60),
					truncate(strings.Join(r.authors, ", "), 60))
			}
			if mud.tags {
				fmt.Printf("      tags   antes:  %s\n      tags   depois: %s\n",
					truncate(strings.Join(r.tagsAtuais, ", "), 60),
					truncate(strings.Join(r.tags, ", "), 60))
			}
		}
		fmt.Printf("\nNADA FOI MODIFICADO (execução de inspeção). Use -apply para aplicar.\n")
		return nil
	}

	var ok, falhas atomic.Int64
	var wg2 sync.WaitGroup
	sem2 := make(chan struct{}, o.Workers)
	start := time.Now()
	for i := range pendentes {
		wg2.Add(1)
		go func(r linha, mud mudanca) {
			defer wg2.Done()
			sem2 <- struct{}{}
			defer func() { <-sem2 }()

			// Uma chamada só para os dois campos: o ebook-meta reescreve o arquivo
			// inteiro a cada invocação, então separar dobraria o custo sem ganho.
			var args []string
			if mud.tags {
				args = append(args, "--tags", strings.Join(r.tags, ","))
			}
			if mud.authors {
				args = append(args, "--authors", strings.Join(r.authors, " & "))
			}
			if err := writeMeta(r.path, args); err != nil {
				falhas.Add(1)
				return
			}
			ok.Add(1)
		}(pendentes[i], mudancas[i])
	}
	wg2.Wait()

	fmt.Printf("\naplicado em %s\n  livros atualizados: %d\n  falhas: %d\n",
		time.Since(start).Round(time.Second), ok.Load(), falhas.Load())
	if ok.Load() > 0 {
		fmt.Println("\nos arquivos mudaram: rode 'robooks index' para atualizar o índice de conteúdo.")
	}
	return nil
}

// lerImportTSV aceita as duas formas descritas em ImportOptions. O segundo retorno diz
// se a coluna de autores veio, para o relatório não prometer o que não vai fazer.
func lerImportTSV(path string) ([]linha, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, fmt.Errorf("abrindo %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	// Cabeçalho opcional: a primeira coluna precisa se chamar "path" para não confundir
	// um caminho de arquivo com um nome de coluna.
	colTags, colAuthors := 1, -1
	primeira := true
	temCabecalho := false

	var out []linha
	for sc.Scan() {
		txt := sc.Text()
		if strings.TrimSpace(txt) == "" {
			continue
		}
		campos := strings.Split(txt, "\t")

		if primeira {
			primeira = false
			if strings.EqualFold(strings.TrimSpace(campos[0]), "path") {
				temCabecalho = true
				colTags, colAuthors = -1, -1
				for i, c := range campos {
					switch strings.ToLower(strings.TrimSpace(c)) {
					case "tags", "genres", "categories":
						colTags = i
					case "authors", "author":
						colAuthors = i
					}
				}
				continue
			}
		}

		p := strings.TrimSpace(campos[0])
		if p == "" {
			continue
		}
		l := linha{path: p}
		if colTags >= 0 && colTags < len(campos) {
			l.tags = listaSeparada(campos[colTags], ",")
			sort.Strings(l.tags)
		}
		if colAuthors >= 0 && colAuthors < len(campos) {
			l.authors = listaSeparada(campos[colAuthors], "&")
		}
		if len(l.tags) == 0 && len(l.authors) == 0 {
			continue
		}
		out = append(out, l)
	}
	return out, temCabecalho && colAuthors >= 0, sc.Err()
}

// listaSeparada quebra o campo e normaliza cada valor.
//
// O Collapse não é enfeite: epub.ReadMeta já colapsa espaços ao ler o OPF, então sem o
// mesmo tratamento aqui a comparação ficaria entre um lado normalizado e outro não.
// "Albert  Camus" no catálogo pareceria diferente de "Albert  Camus" no arquivo, e o
// comando proporia reescrever centenas de epubs para gravar exatamente o que já estava
// lá.
func listaSeparada(s, sep string) []string {
	var out []string
	vistos := map[string]bool{}
	for _, t := range strings.Split(s, sep) {
		t = meta.Collapse(t)
		if t == "" || vistos[strings.ToLower(t)] {
			continue
		}
		vistos[strings.ToLower(t)] = true
		out = append(out, t)
	}
	return out
}

func uniaoOrdenada(a, b []string) []string {
	out := listaSeparada(strings.Join(append(append([]string{}, a...), b...), "\x00"), "\x00")
	sort.Strings(out)
	return out
}
