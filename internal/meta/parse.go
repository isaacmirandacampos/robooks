package meta

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Book é o resultado de interpretar um nome de arquivo da biblioteca. Os nomes
// seguem, com variações, o padrão "Título - Autor.epub", às vezes com a série
// prefixada em colchetes ou parênteses.
type Book struct {
	File   string // nome original com extensão
	Title  string // título limpo
	Author string // autor como aparece no nome
	Series string // série detectada ("" se nenhuma)
	Index  float64
	HasIdx bool
	Dup    bool // nome terminava em "(N)": provável duplicata

	// PrefixNum guarda o "1." / "0.5." que abria o nome, quando o número veio de um
	// prefixo solto sem nome de série. Se no fim nenhuma série for atribuída, ele volta
	// ao nome do arquivo: um "0.5" sem série ainda ordena a novela antes do volume 1,
	// e descartá-lo perderia informação.
	PrefixNum string
}

var (
	// "(Oficial)", "(Dig)" e afins: marcas de origem do arquivo, sem valor descritivo.
	reJunk = regexp.MustCompile(`(?i)\s*\((?:oficial|dig|digital|rev|revisado)\)\s*`)
	// Assinatura de site de download entre parênteses ou colchetes, como
	// "(z-library.sk, 1lib.sk, z-lib.sk)". Exige um domínio de verdade dentro para não
	// comer parênteses legítimos: "(Portuguese Edition)", "(Vol. 2)" e "(Ed. 34)"
	// continuam intactos porque nenhum tem letra+ponto+TLD.
	reSiteTag = regexp.MustCompile(`(?i)\s*[\(\[][^)\]]*\b[a-z0-9-]+\.(?:com|net|org|sk|to|se|ru|io|me|info|xyz|cc|is|st)\b[^)\]]*[\)\]]`)
	// Sites conhecidos que às vezes aparecem sem domínio.
	reKnownSite = regexp.MustCompile(`(?i)\s*[\(\[]\s*(?:z-?library|zlib|libgen|library genesis|anna'?s archive|epub ?reader|le livros|lelivros|book ?zz|b-ok)\s*[\)\]]`)
	// Sufixo "(1)", "(2)": cópias criadas pelo download, não parte do título.
	reDupSuffix = regexp.MustCompile(`\s*\((\d+)\)\s*$`)

	// Série entre colchetes. O número pode estar solto ("[Maze Runner 2]") ou colado
	// ("[Abandono02]"), e o fecha-colchete pode vir sem espaço antes do título.
	reBracket = regexp.MustCompile(`^\[\s*(.+?)\s*(\d{1,3})\s*\]\s*(.*)$`)
	// Série entre colchetes sem número nenhum: "[Thrawn01]" já cai no caso acima,
	// mas "[Wicked]" não tem volume a extrair.
	reBracketNoNum = regexp.MustCompile(`^\[\s*([^\]]+?)\s*\]\s*(.*)$`)
	// Série entre parênteses: "(Magisterium 2)", "(Mortal #24)", "(Mar despedacado#1)".
	reParen = regexp.MustCompile(`^\(\s*(.+?)\s*#?\s*(\d{1,3})\s*\)\s*(.*)$`)
	// Prefixo numérico de volume: "1. A Flor da Pele", "0.5. Doce Tatuagem". Exige o
	// ponto depois do número: sem ele, "666 - O Limiar do Inferno" e "1822 - Laurentino
	// Gomes" seriam lidos como volume 666 e 1822, comendo parte do título.
	rePrefixNum = regexp.MustCompile(`^(\d{1,2}(?:\.\d)?)\.\s+(.*)$`)
	// "Vol"/"Volume" preso ao fim do nome da série: "(The 100 Vol. 3)" -> "The 100".
	reSeriesVolTail = regexp.MustCompile(`(?i)\s*[-–]?\s*vol(?:ume)?\.?\s*$`)
	// Sufixo de volume: "Ramses - Vol 3", "Vampiro-Rei - Vol.2".
	reVolSuffix = regexp.MustCompile(`(?i)^(.*?)\s*[-–]\s*(.+?)\s*[-–]?\s*Vol\.?\s*(\d{1,3})\s*$`)
	// "Titulo - Vol. N" sem nome de série separado.
	reVolOnly = regexp.MustCompile(`(?i)^(.*?)\s*[-–]\s*Vol\.?\s*(\d{1,3})\s*$`)
	// "House of Night 01 - Marcada", "Isaac Asimov Magazine 01"
	reSeriesNumDash = regexp.MustCompile(`^(.*?[^\d\s])\s+(\d{1,3})\s*[-–]\s*(.+)$`)
)

// mesesPT mapeia nomes de mês para índice, para coleções organizadas por mês
// (a biblioteca tem "A Garota do Calendário — Janeiro..Dezembro").
var mesesPT = map[string]float64{
	"janeiro": 1, "fevereiro": 2, "marco": 3, "março": 3, "abril": 4,
	"maio": 5, "junho": 6, "julho": 7, "agosto": 8, "setembro": 9,
	"outubro": 10, "novembro": 11, "dezembro": 12,
}

// Parse interpreta um nome de arquivo (sem extensão) em título, autor e série.
// Não faz clustering — isso depende do conjunto todo e acontece depois.
func Parse(stem string) Book { return parse(stem, true) }

// ParseTitleOnly aplica as mesmas limpezas de série e sufixos, mas nunca tenta
// separar autor. Serve para o dc:Title interno do epub, que é só o título: ali
// "666 - O LIMIAR DO INFERNO" tem um " - " que não separa autor nenhum, e tratá-lo
// como separador reduziria o título a "666".
func ParseTitleOnly(s string) Book { return parse(s, false) }

func parse(stem string, extractAuthor bool) Book {
	b := Book{}

	// O sufixo de duplicata sai primeiro para não ser confundido com volume, mas
	// fica registrado: o usuário quer saber quais são, sem que nada seja apagado.
	if m := reDupSuffix.FindStringSubmatch(stem); m != nil {
		b.Dup = true
		stem = reDupSuffix.ReplaceAllString(stem, "")
	}
	stem = reSiteTag.ReplaceAllString(stem, " ")
	stem = reKnownSite.ReplaceAllString(stem, " ")
	stem = reJunk.ReplaceAllString(stem, " ")
	stem = strings.TrimSpace(Collapse(stem))

	// Autor é o que vem depois do último " - ". Só aceita se parecer nome de pessoa:
	// curto e sem dígitos, senão "Ramses - Vol 3" viria com autor "Vol 3".
	Title := stem
	if extractAuthor {
		if i := strings.LastIndex(stem, " - "); i > 0 {
			cand := strings.TrimSpace(stem[i+3:])
			if looksLikeAuthor(cand) {
				b.Author = cand
				Title = strings.TrimSpace(stem[:i])
			}
		}
	}

	// A ordem importa: os padrões mais específicos primeiro, senão "[Serie 2]" cairia
	// no caso sem número e perderia o volume.
	switch {
	case reBracket.MatchString(Title):
		m := reBracket.FindStringSubmatch(Title)
		b.Series, b.Index, b.HasIdx = clean(m[1]), atof(m[2]), true
		Title = m[3]
	case reParen.MatchString(Title):
		m := reParen.FindStringSubmatch(Title)
		b.Series, b.Index, b.HasIdx = clean(m[1]), atof(m[2]), true
		Title = m[3]
	case reBracketNoNum.MatchString(Title):
		m := reBracketNoNum.FindStringSubmatch(Title)
		if m[2] != "" { // "[Serie]" sozinho não é série, é título entre colchetes
			b.Series = clean(m[1])
			Title = m[2]
		}
	case reVolSuffix.MatchString(Title):
		m := reVolSuffix.FindStringSubmatch(Title)
		b.Series, b.Index, b.HasIdx = clean(m[2]), atof(m[3]), true
		Title = m[1]
	case reVolOnly.MatchString(Title):
		m := reVolOnly.FindStringSubmatch(Title)
		b.Index, b.HasIdx = atof(m[2]), true
		Title = m[1]
	case reSeriesNumDash.MatchString(Title):
		m := reSeriesNumDash.FindStringSubmatch(Title)
		b.Series, b.Index, b.HasIdx = clean(m[1]), atof(m[2]), true
		Title = m[3]
	case rePrefixNum.MatchString(Title):
		// Só um número solto na frente: dá o volume, mas não o nome da série. O
		// clustering preenche o nome depois, se houver irmãos do mesmo autor.
		m := rePrefixNum.FindStringSubmatch(Title)
		b.Index, b.HasIdx = atof(m[1]), true
		b.PrefixNum = m[1]
		Title = m[2]
	}

	b.Series = reSeriesVolTail.ReplaceAllString(b.Series, "")
	b.Title = clean(Title)
	return b
}

// looksLikeAuthor evita tratar como autor um fragmento que é claramente parte do
// título (contém dígitos, é longo demais, ou é uma palavra funcional).
func looksLikeAuthor(s string) bool {
	if s == "" || len(s) > 45 {
		return false
	}
	if strings.ContainsAny(s, "0123456789") {
		return false
	}
	low := strings.ToLower(s)
	for _, bad := range []string{"vol", "volume", "parte", "livro", "tomo", "ed.", "edicao"} {
		if low == bad || strings.HasPrefix(low, bad+" ") {
			return false
		}
	}
	return true
}

func clean(s string) string {
	s = strings.Trim(Collapse(s), " -–_.,")
	return s
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// FmtIndex escreve o volume com dois dígitos, para que a ordenação alfabética no
// gerenciador de arquivos coincida com a ordem de leitura (02 antes de 10).
func FmtIndex(f float64) string {
	if f == float64(int(f)) {
		return fmt.Sprintf("%02d", int(f))
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// NewFilename monta o nome final no formato escolhido: "Série NN - Título - Autor".
func (b Book) NewFilename() string {
	Title := b.Title
	// Títulos gritados vão para caixa mista também no nome do arquivo, não só no
	// metadado — foi o que o usuário pediu ao normalizar as maiúsculas.
	if IsShouty(Title) {
		Title = TitleCasePT(Title)
	}

	var parts []string
	switch {
	case b.Series != "" && b.HasIdx:
		parts = append(parts, b.Series+" "+FmtIndex(b.Index))
	case b.Series != "":
		parts = append(parts, b.Series)
	case b.PrefixNum != "":
		// Nenhuma série foi determinada: devolve o prefixo numérico ao título em vez
		// de descartá-lo.
		Title = b.PrefixNum + ". " + Title
	}
	parts = append(parts, Title)
	if b.Author != "" {
		parts = append(parts, b.Author)
	}
	name := strings.Join(parts, " - ")
	if b.Dup {
		// Preserva a marca de duplicata: o usuário pediu para só relatar, então os
		// pares precisam continuar distinguíveis.
		name += " (1)"
	}
	return Sanitize(name) + ".epub"
}

// Sanitize remove o que quebra nome de arquivo em Linux ou em compartilhamento
// Windows/Samba — inclusive ":" , que é justamente por isso que a biblioteca usa "_".
var reBadChars = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]`)

func Sanitize(s string) string {
	s = reBadChars.ReplaceAllString(s, "")
	s = Collapse(s)
	if len(s) > 200 { // margem sob o limite de 255 bytes do ext4
		s = strings.TrimSpace(s[:200])
	}
	return s
}
