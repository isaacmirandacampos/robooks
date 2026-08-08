package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/isaacmirandacampos/robooks/internal/authors"
)

// cmdAuthors procura autores duplicados numa lista vinda do catálogo.
//
// A entrada é TSV pela entrada padrão — "id<TAB>livros<TAB>nome" — em vez de uma conexão
// com o banco. O robooks não fala com servidor nenhum em nenhum outro comando, e abrir
// exceção aqui traria um driver de banco para dentro de um binário que hoje não tem
// dependência externa. Qualquer catálogo consegue produzir essas três colunas, então o
// comando serve a todos eles sem conhecer nenhum:
//
//	docker exec grimmory-mariadb mariadb -N -B grimmory -e '<consulta>' | robooks authors
func cmdAuthors(args []string) int {
	fs := flag.NewFlagSet("authors", flag.ExitOnError)
	typos := fs.Bool("typos", true, "também procurar erros de digitação (revise antes de aplicar)")
	minLen := fs.Int("min-len-typo", 12, "tamanho mínimo do nome para admitir diferença de uma letra")
	soConfiavel := fs.Bool("safe", false, "listar apenas os grupos que dispensam revisão")
	sqlOut := fs.Bool("sql", false, "emitir o SQL de merge para o Grimmory em vez do relatório")
	limite := fs.Int("n", 0, "mostrar no máximo N grupos (0 = todos)")
	skip := fs.String("skip", "", "ids separados por vírgula que não devem entrar em grupo nenhum")
	keep := fs.String("keep", "", "ids separados por vírgula que devem sobreviver ao merge do seu grupo")
	fs.Parse(args)

	vetados, forcados := listaIDs(*skip), listaIDs(*keep)

	linhas, err := lerTSV(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro lendo a entrada: %v\n", err)
		return 1
	}
	if len(linhas) == 0 {
		fmt.Fprintln(os.Stderr, "nenhum registro na entrada padrão.\n"+
			"uso: <consulta ao catálogo> | robooks authors")
		return 2
	}
	recs := make([]authors.Record, len(linhas))
	for i, l := range linhas {
		recs[i] = authors.Record{ID: l.ID, Books: l.Books, Name: l.Name}
	}

	grupos := authors.Analyze(recs, authors.Options{
		Typos: *typos, MinLenTypo: *minLen, Skip: vetados, Keep: forcados,
	})
	if *soConfiavel {
		var f []authors.Group
		for _, g := range grupos {
			if g.Confiavel() {
				f = append(f, g)
			}
		}
		grupos = f
	}

	if *sqlOut {
		emitirSQL(grupos)
		return 0
	}
	relatorio(recs, grupos, *limite, *soConfiavel)
	return 0
}

func listaIDs(s string) map[string]bool {
	out := map[string]bool{}
	for _, id := range strings.Split(s, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

// catalogRecord é uma linha do TSV que os comandos authors e genres consomem:
// identificador, quantos livros usam a entrada, e o nome. É o menor denominador comum
// entre os catálogos — todos sabem produzir estas três colunas.
type catalogRecord struct {
	ID    string
	Books int
	Name  string
}

func lerTSV(f *os.File) ([]catalogRecord, error) {
	var out []catalogRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		linha := sc.Text()
		if strings.TrimSpace(linha) == "" {
			continue
		}
		campos := strings.Split(linha, "\t")
		if len(campos) < 3 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(campos[1]))
		if err != nil {
			continue // cabeçalho ou linha decorativa
		}
		nome := strings.TrimSpace(strings.Join(campos[2:], "\t"))
		if nome == "" || nome == "NULL" {
			continue
		}
		out = append(out, catalogRecord{ID: strings.TrimSpace(campos[0]), Books: n, Name: nome})
	}
	return out, sc.Err()
}

func relatorio(recs []authors.Record, grupos []authors.Group, limite int, soConfiavel bool) {
	confiaveis, revisar, absorvidos := 0, 0, 0
	for _, g := range grupos {
		absorvidos += len(g.Others)
		if g.Confiavel() {
			confiaveis++
		} else {
			revisar++
		}
	}

	fmt.Printf("%d autores no catálogo, %d grupos duplicados, %d registros a absorver\n\n",
		len(recs), len(grupos), absorvidos)

	mostrados := 0
	for _, g := range grupos {
		if limite > 0 && mostrados >= limite {
			fmt.Printf("... e mais %d grupos (use -n 0 para ver todos)\n", len(grupos)-mostrados)
			break
		}
		mostrados++

		marca := " "
		if !g.Confiavel() {
			marca = "?" // pede olho humano
		}
		var rs []string
		for _, r := range g.Rules {
			rs = append(rs, string(r))
		}
		fmt.Printf("%s %s  [%s, %d livros]\n", marca, g.Canonical.Name,
			strings.Join(rs, "+"), g.Books)
		fmt.Printf("    manter  #%-6s %3d  %s\n", g.Canonical.ID, g.Canonical.Books, g.Canonical.Name)
		for _, o := range g.Others {
			fmt.Printf("    juntar  #%-6s %3d  %s\n", o.ID, o.Books, o.Name)
		}
		fmt.Println()
	}

	fmt.Printf("resumo: %d grupos seguros, %d marcados com ? para revisão\n", confiaveis, revisar)
	if !soConfiavel && revisar > 0 {
		fmt.Println("        -safe esconde os que precisam de revisão; -sql emite o merge")
	}
}

// emitirSQL escreve a transação de merge.
//
// O update pode deixar o mesmo autor duas vezes no mesmo livro, porque a chave primária
// do mapeamento é (book_id, sort_order) e não (book_id, author_id): se um livro já citava
// as duas grafias, ele passa a citar a mesma pessoa em duas posições. O delete seguinte
// remove a repetição, mantendo a de menor sort_order.
func emitirSQL(grupos []authors.Group) {
	var absorvidos []string
	fmt.Println("-- merge de autores duplicados, gerado por: robooks authors -sql")
	fmt.Println("-- confira o relatório antes de aplicar. faça backup do banco.")
	fmt.Println("START TRANSACTION;")
	fmt.Println()

	for _, g := range grupos {
		var idsOutros []string
		for _, o := range g.Others {
			idsOutros = append(idsOutros, o.ID)
			absorvidos = append(absorvidos, o.ID)
		}
		if len(idsOutros) == 0 {
			continue
		}
		fmt.Printf("-- %s  <-  %s\n", g.Canonical.Name, nomesDe(g.Others))
		fmt.Printf("UPDATE book_metadata_author_mapping SET author_id = %s WHERE author_id IN (%s);\n",
			g.Canonical.ID, strings.Join(idsOutros, ", "))
	}

	if len(absorvidos) > 0 {
		sort.Strings(absorvidos)
		fmt.Println()
		fmt.Println("-- remove a repetição criada quando um livro já citava duas grafias do mesmo autor")
		fmt.Println("DELETE m1 FROM book_metadata_author_mapping m1")
		fmt.Println("  JOIN book_metadata_author_mapping m2")
		fmt.Println("    ON m1.book_id = m2.book_id AND m1.author_id = m2.author_id")
		fmt.Println("   AND m1.sort_order > m2.sort_order;")
		fmt.Println()
		fmt.Printf("DELETE FROM author WHERE id IN (%s);\n", strings.Join(absorvidos, ", "))
	}

	fmt.Println()
	fmt.Println("COMMIT;")
}

func nomesDe(rs []authors.Record) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return strings.Join(out, ", ")
}
