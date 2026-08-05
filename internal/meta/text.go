package meta

import (
	"strings"
	"unicode"
)

// NormKey reduz um texto à forma comparável: minúsculas, sem acento, apenas letras e
// dígitos ASCII.
//
// Descartar o que não é ASCII depois de remover o acento é essencial, não cosmético:
// "2° chance" (símbolo de grau, U+00B0) e "2º chance" (ordinal masculino, U+00BA) são o
// mesmo livro escrito com caracteres diferentes, e os dois passam por unicode.IsLetter.
// Preservá-los produz chaves distintas e o par escapa da deduplicação.
func NormKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if d := Deaccent(r); (d >= 'a' && d <= 'z') || (d >= '0' && d <= '9') {
				b.WriteRune(d)
			}
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}
	return Collapse(b.String())
}

// Collapse normaliza espaços em branco consecutivos para um único espaço.
func Collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

const accentFrom = "áàâãäéèêëíìîïóòôõöúùûüçñ"
const accentTo = "aaaaaeeeeiiiiooooouuuucn"

// Deaccent remove o acento de uma runa minúscula; devolve a runa original se não houver
// equivalente conhecido.
func Deaccent(r rune) rune {
	if i := strings.IndexRune(accentFrom, r); i >= 0 {
		return []rune(accentTo)[len([]rune(accentFrom[:i]))]
	}
	return r
}

// DeaccentStr remove acentos preservando a caixa original.
func DeaccentStr(s string) string {
	var b strings.Builder
	for _, r := range s {
		d := Deaccent(unicode.ToLower(r))
		if unicode.IsUpper(r) {
			d = unicode.ToUpper(d)
		}
		b.WriteRune(d)
	}
	return b.String()
}

// Tokenize reduz texto livre a palavras minúsculas sem acento nem pontuação. É a base
// da assinatura de conteúdo: diferenças de formatação entre conversões não podem
// alterar o resultado.
func Tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			d := Deaccent(unicode.ToLower(r))
			if (d >= 'a' && d <= 'z') || (d >= '0' && d <= '9') {
				cur.WriteRune(d)
			}
		default:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
