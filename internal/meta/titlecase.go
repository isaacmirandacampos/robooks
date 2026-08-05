package meta

import (
	"regexp"
	"strings"
	"unicode"
)

// minusculasPT são as palavras funcionais que ficam minúsculas no meio de um título
// em português. A primeira e a última palavra são sempre capitalizadas.
var minusculasPT = map[string]bool{
	"a": true, "as": true, "o": true, "os": true, "um": true, "uma": true,
	"uns": true, "umas": true, "de": true, "da": true, "do": true, "das": true,
	"dos": true, "e": true, "em": true, "na": true, "no": true, "nas": true,
	"nos": true, "num": true, "numa": true, "por": true, "para": true, "pra": true,
	"com": true, "sem": true, "sob": true, "sobre": true, "ao": true, "aos": true,
	"à": true, "às": true, "que": true, "se": true, "ou": true, "nem": true,
	"pelo": true, "pela": true, "até": true, "entre": true, "contra": true,
	"desde": true, "após": true, "ante": true, "the": true, "of": true, "and": true,
}

// siglas que devem permanecer em caixa alta ao normalizar um título gritado.
var siglas = map[string]bool{
	"eua": true, "urss": true, "brasil": true, "onu": true, "otan": true,
	"cia": true, "fbi": true, "kgb": true, "url": true, "html": true, "css": true,
	"sql": true, "php": true, "xml": true, "api": true, "ia": true, "ai": true,
	"tv": true, "dna": true, "hiv": true, "aids": true, "ufo": true, "nasa": true,
	"jfk": true, "usa": true, "uk": true, "mpb": true, "oab": true, "stf": true,
	"pt": true, "psdb": true, "url2": false,
}

var reWordSplit = regexp.MustCompile(`([^\p{L}\p{N}']+)`)

// IsShouty reconhece um título escrito inteiramente em caixa alta. Exige um mínimo
// de letras para não confundir siglas curtas legítimas ("EUA") com um título gritado.
func IsShouty(s string) bool {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters < 8 {
		return false
	}
	// Tolera uma minúscula perdida (ex: "O LIMIAR DO INFERNo").
	return float64(upper)/float64(letters) >= 0.95
}

// TitleCasePT normaliza um título em caixa alta. Só deve ser chamado quando
// IsShouty é verdadeiro: aplicar isso a um título já bem escrito destruiria
// capitalização intencional de nomes próprios.
func TitleCasePT(s string) string {
	parts := reWordSplit.Split(s, -1)
	seps := reWordSplit.FindAllString(s, -1)

	// Índices das palavras reais (não vazias), para saber qual é a última.
	var idx []int
	for i, p := range parts {
		if p != "" {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		return s
	}
	first, last := idx[0], idx[len(idx)-1]

	for i, p := range parts {
		if p == "" {
			continue
		}
		low := strings.ToLower(p)
		// Uma palavra funcional volta a maiúscula quando abre um segmento — depois de
		// travessão, dois-pontos ou ponto ("666 - O Limiar do Inferno").
		startsSegment := i == first || (i > 0 && i-1 < len(seps) && strings.ContainsAny(seps[i-1], "-–—:.?!;()[]/"))
		switch {
		case siglas[low]:
			parts[i] = strings.ToUpper(p)
		case isRoman(low):
			parts[i] = strings.ToUpper(p)
		case !startsSegment && i != last && minusculasPT[low]:
			parts[i] = low
		default:
			parts[i] = capitalize(low)
		}
	}

	var sb strings.Builder
	for i, p := range parts {
		sb.WriteString(p)
		if i < len(seps) {
			sb.WriteString(seps[i])
		}
	}
	return sb.String()
}

// isRoman evita rebaixar numerais romanos ("Guerra Mundial II") a "Ii".
func isRoman(s string) bool {
	if s == "" || len(s) > 6 {
		return false
	}
	// "i" e "mi" isolados são palavras/ruído mais provavelmente que numerais.
	if s == "i" || s == "mi" || s == "di" || s == "li" || s == "ci" {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("ivxlcdm", r) {
			return false
		}
	}
	return true
}

func capitalize(s string) string {
	rs := []rune(s)
	if len(rs) == 0 {
		return s
	}
	rs[0] = unicode.ToUpper(rs[0])
	return string(rs)
}

// RestoreColon devolve o ":" que foi trocado por "_" para o título caber em sistemas
// de arquivos Windows. Só se aplica ao metadado dc:Title — no nome do arquivo o "_"
// permanece, senão o compartilhamento Samba/Windows quebra.
//
// O padrão alvo é "_" seguido de espaço ("13 Horas_ Os Soldados"), que é como a
// substituição foi feita. Um "_" entre palavras sem espaço (snake_case) fica intacto.
var reUnderscoreColon = regexp.MustCompile(`(\p{L}|\p{N}|["')\]])_(\s)`)

func RestoreColon(s string) string {
	return reUnderscoreColon.ReplaceAllString(s, "$1:$2")
}

// AuthorFileAs converte "J. R. R. Tolkien" em "Tolkien, J. R. R." para o atributo
// opf:file-as, que é o que ordena a lista de autores. Vários epubs vieram do MOBI
// com file-as="Unknown".
func AuthorFileAs(name string) string {
	name = Collapse(name)
	if name == "" || strings.Contains(name, ",") {
		return name
	}
	// Nomes com múltiplos autores não têm forma canônica única; deixa como está.
	for _, sep := range []string{" e ", " & ", ";", " and "} {
		if strings.Contains(strings.ToLower(name), sep) {
			return name
		}
	}
	fields := strings.Fields(name)
	if len(fields) < 2 {
		return name
	}
	surname := fields[len(fields)-1]
	// Partículas de sobrenome acompanham o sobrenome: "Alberto da Costa e Silva".
	i := len(fields) - 1
	for i > 1 {
		p := strings.ToLower(fields[i-1])
		if p == "de" || p == "da" || p == "do" || p == "dos" || p == "das" || p == "van" || p == "von" || p == "del" || p == "la" {
			i--
			surname = fields[i] + " " + surname
			continue
		}
		break
	}
	rest := strings.Join(fields[:i], " ")
	if rest == "" {
		return name
	}
	return surname + ", " + rest
}
