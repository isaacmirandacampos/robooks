package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/isaacmirandacampos/robooks/internal/genre"
)

// cmdGenres limpa o vocabulário de gêneros de um catálogo.
//
// Lê o mesmo TSV que o comando authors — "id<TAB>livros<TAB>nome" — pelas mesmas razões:
// o robooks não abre conexão com servidor nenhum, e as três colunas são o que qualquer
// catálogo sabe exportar.
//
// A saída descreve três destinos para cada rótulo: virar outro (canonizar), sumir
// (ruído) ou ficar como está.
func cmdGenres(args []string) int {
	fs := flag.NewFlagSet("genres", flag.ExitOnError)
	minFreq := fs.Int("min", 3, "gêneros fora do vocabulário conhecido com menos de N livros são removidos")
	sqlOut := fs.Bool("sql", false, "emitir o SQL de limpeza para o Grimmory")
	verbose := fs.Bool("v", false, "listar todos os rótulos, não só o resumo")
	keep := fs.String("keep", "", "rótulos separados por vírgula a preservar mesmo abaixo de -min")
	nomesPath := fs.String("names", "", "arquivo com um nome por linha (autores); rótulos iguais a eles são removidos")
	fs.Parse(args)

	nomes := map[string]bool{}
	if *nomesPath != "" {
		b, err := os.ReadFile(*nomesPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "erro lendo -names: %v\n", err)
			return 1
		}
		for _, l := range strings.Split(string(b), "\n") {
			if l = strings.TrimSpace(strings.ToLower(l)); l != "" {
				nomes[l] = true
			}
		}
	}

	preservar := map[string]bool{}
	for _, g := range strings.Split(*keep, ",") {
		if g = strings.TrimSpace(strings.ToLower(g)); g != "" {
			preservar[g] = true
		}
	}

	recs, err := lerTSV(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro lendo a entrada: %v\n", err)
		return 1
	}
	if len(recs) == 0 {
		fmt.Fprintln(os.Stderr, "nenhum registro na entrada padrão.\n"+
			"uso: <consulta ao catálogo> | robooks genres")
		return 2
	}

	plano := planejar(recs, *minFreq, preservar, nomes)
	if *sqlOut {
		emitirSQLGeneros(plano)
		return 0
	}
	relatorioGeneros(plano, *verbose)
	return 0
}

// destino é o que acontece com um rótulo do catálogo.
type destino struct {
	de     string // rótulo original
	id     string
	livros int
	para   string // "" quando o rótulo deve ser removido
	motivo string
}

type planoGeneros struct {
	itens []destino
	// depois mapeia o rótulo final ao total de livros que passam a apontar para ele.
	depois map[string]int
	antes  int
}

func planejar(recs []catalogRecord, minFreq int, preservar, nomes map[string]bool) planoGeneros {
	p := planoGeneros{depois: map[string]int{}, antes: len(recs)}

	for _, r := range recs {
		d := destino{de: r.Name, id: r.ID, livros: r.Books}

		// Nome de autor não é gênero, e nenhuma regra estrutural o distingue de um
		// rótulo legítimo: "Agatha Christie" tem duas palavras capitalizadas e nada de
		// suspeito. Só o próprio catálogo sabe responder.
		if genre.IsPersonName(r.Name, nomes) {
			d.motivo = "nome de autor"
			p.itens = append(p.itens, d)
			continue
		}

		limpo := genre.CleanEN([]string{r.Name})
		switch {
		case len(limpo) == 0:
			// A limpeza rejeitou: ruído, formato, lugar, ou rótulo mapeado para descarte.
			d.motivo = "ruído"
		case len(limpo) > 1:
			// Hierarquia colada — "Fantasy; Ghosts; Suspense" — vira o primeiro termo.
			// Abrir em vários exigiria criar mapeamentos novos, não só redirecionar.
			d.para = limpo[0]
			d.motivo = "lista"
		default:
			d.para = limpo[0]
		}

		if d.para != "" {
			_, conhecido := genre.CanonEN(r.Name)
			// Fora do vocabulário e raro: é cauda longa de catalogação, não filtro.
			if !conhecido && r.Books < minFreq && !preservar[strings.ToLower(r.Name)] {
				d.para, d.motivo = "", fmt.Sprintf("raro (<%d)", minFreq)
			}
		}
		if d.para != "" && d.motivo == "" && !strings.EqualFold(d.para, r.Name) {
			d.motivo = "canoniza"
		}
		if d.para != "" {
			p.depois[d.para] += d.livros
		}
		p.itens = append(p.itens, d)
	}
	return p
}

func relatorioGeneros(p planoGeneros, verbose bool) {
	var removidos, canonizados, mantidos int
	livrosPerdidos := 0
	porMotivo := map[string]int{}

	for _, d := range p.itens {
		switch {
		case d.para == "":
			removidos++
			livrosPerdidos += d.livros
			porMotivo[d.motivo]++
		case d.motivo != "":
			canonizados++
		default:
			mantidos++
		}
	}

	fmt.Printf("%d gêneros no catálogo  →  %d depois da limpeza\n\n", p.antes, len(p.depois))
	fmt.Printf("  %5d removidos    (%d associações de livro desfeitas)\n", removidos, livrosPerdidos)
	for _, m := range chavesOrdenadas(porMotivo) {
		fmt.Printf("        %-14s %d\n", m, porMotivo[m])
	}
	fmt.Printf("  %5d canonizados  (viram outro rótulo já existente)\n", canonizados)
	fmt.Printf("  %5d inalterados\n\n", mantidos)

	fmt.Println("os 30 maiores depois da limpeza:")
	type kv struct {
		g string
		n int
	}
	var top []kv
	for g, n := range p.depois {
		top = append(top, kv{g, n})
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].n != top[j].n {
			return top[i].n > top[j].n
		}
		return top[i].g < top[j].g
	})
	for i, t := range top {
		if i >= 30 {
			break
		}
		fmt.Printf("  %5d  %s\n", t.n, t.g)
	}

	if !verbose {
		fmt.Println("\nuse -v para ver rótulo a rótulo, -sql para aplicar")
		return
	}

	fmt.Println("\n--- rótulo a rótulo ---")
	itens := append([]destino(nil), p.itens...)
	sort.Slice(itens, func(i, j int) bool { return itens[i].livros > itens[j].livros })
	for _, d := range itens {
		switch {
		case d.para == "":
			fmt.Printf("  %5d  %-40s  REMOVE (%s)\n", d.livros, d.de, d.motivo)
		case d.motivo != "":
			fmt.Printf("  %5d  %-40s  →  %s\n", d.livros, d.de, d.para)
		}
	}
}

func chavesOrdenadas(m map[string]int) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// emitirSQLGeneros escreve a limpeza como transação.
//
// Cada rótulo que muda de nome precisa apontar para a linha de category que já tem o
// nome de destino, criando-a se ainda não existir; o mapeamento é então redirecionado e
// a linha antiga apagada. A chave primária de book_metadata_category_mapping é o par
// (book_id, category_id), então o redirecionamento pode colidir quando o livro já tinha
// os dois rótulos — daí o INSERT IGNORE seguido de DELETE, em vez de UPDATE.
func emitirSQLGeneros(p planoGeneros) {
	fmt.Println("-- limpeza de gêneros, gerada por: robooks genres -sql")
	fmt.Println("-- confira o relatório antes de aplicar. faça backup do banco.")
	fmt.Println("START TRANSACTION;")
	fmt.Println()

	// Garante que todo rótulo de destino exista como linha.
	var destinos []string
	for g := range p.depois {
		destinos = append(destinos, g)
	}
	sort.Strings(destinos)
	fmt.Println("-- 1. cria os rótulos de destino que ainda não existem")
	for _, g := range destinos {
		fmt.Printf("INSERT IGNORE INTO category (name) VALUES (%s);\n", quoteSQL(g))
	}

	fmt.Println()
	fmt.Println("-- 2. redireciona os mapeamentos dos rótulos que mudam de nome")
	for _, d := range p.itens {
		if d.para == "" || strings.EqualFold(d.para, d.de) {
			continue
		}
		fmt.Printf("INSERT IGNORE INTO book_metadata_category_mapping (book_id, category_id)\n"+
			"  SELECT m.book_id, (SELECT id FROM category WHERE name = %s)\n"+
			"    FROM book_metadata_category_mapping m WHERE m.category_id = %s;\n",
			quoteSQL(d.para), d.id)
	}

	fmt.Println()
	fmt.Println("-- 3. apaga os rótulos removidos e os que foram redirecionados")
	var apagar []string
	for _, d := range p.itens {
		if d.para == "" || !strings.EqualFold(d.para, d.de) {
			apagar = append(apagar, d.id)
		}
	}
	// Em lotes: uma cláusula IN com milhares de itens estoura o max_allowed_packet.
	for i := 0; i < len(apagar); i += 500 {
		fim := i + 500
		if fim > len(apagar) {
			fim = len(apagar)
		}
		lote := strings.Join(apagar[i:fim], ", ")
		fmt.Printf("DELETE FROM book_metadata_category_mapping WHERE category_id IN (%s);\n", lote)
		fmt.Printf("DELETE FROM category WHERE id IN (%s);\n", lote)
	}

	fmt.Println()
	fmt.Println("COMMIT;")
}

func quoteSQL(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "'", "''") + "'"
}
