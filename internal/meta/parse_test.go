package meta

import "testing"

func TestParseName(t *testing.T) {
	cases := []struct {
		in      string
		Title   string
		Author  string
		Series  string
		index   float64
		HasIdx  bool
		newName string
	}{
		{
			in:    "[Maze Runner 2]Prova de fogo - James Dashner",
			Title: "Prova de fogo", Author: "James Dashner", Series: "Maze Runner",
			index: 2, HasIdx: true,
			newName: "Maze Runner 02 - Prova de fogo - James Dashner.epub",
		},
		{
			// número colado no nome da série, e sufixo "(Oficial)" para remover
			in:    "[Abandono02]Inferno(Oficial) - Meg Cabot",
			Title: "Inferno", Author: "Meg Cabot", Series: "Abandono",
			index: 2, HasIdx: true,
			newName: "Abandono 02 - Inferno - Meg Cabot.epub",
		},
		{
			in:    "(Magisterium 2) Luva de Cobre - Cassandra Clare",
			Title: "Luva de Cobre", Author: "Cassandra Clare", Series: "Magisterium",
			index: 2, HasIdx: true,
			newName: "Magisterium 02 - Luva de Cobre - Cassandra Clare.epub",
		},
		{
			in:    "(Mortal #24) Inocencia mortal - Nora Roberts",
			Title: "Inocencia mortal", Author: "Nora Roberts", Series: "Mortal",
			index: 24, HasIdx: true,
			newName: "Mortal 24 - Inocencia mortal - Nora Roberts.epub",
		},
		{
			// série no meio, volume no fim
			in:    "A Batalha de Kadesh - Ramses - Vol 3 - Christian Jacq",
			Title: "A Batalha de Kadesh", Author: "Christian Jacq", Series: "Ramses",
			index: 3, HasIdx: true,
			newName: "Ramses 03 - A Batalha de Kadesh - Christian Jacq.epub",
		},
		{
			in:    "House of Night 01 - Marcada - P. C. Cast",
			Title: "Marcada", Author: "P. C. Cast", Series: "House of Night",
			index: 1, HasIdx: true,
			newName: "House of Night 01 - Marcada - P. C. Cast.epub",
		},
		{
			// prefixo numérico sem nome de série: o número volta ao nome, para não
			// perder a ordenação da novela 0.5 antes do volume 1
			in:    "1. A Flor da Pele - Helena Hunting",
			Title: "A Flor da Pele", Author: "Helena Hunting", Series: "",
			index: 1, HasIdx: true,
			newName: "1. A Flor da Pele - Helena Hunting.epub",
		},
		{
			// "666" é parte do título, não volume: exige ponto para ser índice
			in:      "666 - O LIMIAR DO INFERNO - Jay Anson",
			Title:   "666 - O LIMIAR DO INFERNO",
			Author:  "Jay Anson",
			newName: "666 - O Limiar do Inferno - Jay Anson.epub",
		},
		{
			// "Vol." não deve grudar no nome da série
			in:    "(The 100 Vol. 3) De Volta - Kass Morgan",
			Title: "De Volta", Author: "Kass Morgan", Series: "The 100",
			index: 3, HasIdx: true,
			newName: "The 100 03 - De Volta - Kass Morgan.epub",
		},
		{
			// livro avulso: nada deve mudar
			in:      "1984 - George Orwell",
			Title:   "1984",
			Author:  "George Orwell",
			newName: "1984 - George Orwell.epub",
		},
		{
			// "Vol 3" não pode ser confundido com autor
			in:    "A Comedia Humana - Vol. 3 - Honore de Balzac",
			Title: "A Comedia Humana", Author: "Honore de Balzac", Series: "",
			index: 3, HasIdx: true,
			newName: "A Comedia Humana - Honore de Balzac.epub",
		},
		{
			// duplicata: marca preservada, nada apagado
			in:    "1o a Morrer - James Patterson (1)",
			Title: "1o a Morrer", Author: "James Patterson",
			newName: "1o a Morrer - James Patterson (1).epub",
		},
		{
			// título com dois pontos virado "_": nome de arquivo mantém o "_"
			in:      "13 Horas_ Os Soldados Secretos de Benghazi - Mitchell Zuckoff",
			Title:   "13 Horas_ Os Soldados Secretos de Benghazi",
			Author:  "Mitchell Zuckoff",
			newName: "13 Horas_ Os Soldados Secretos de Benghazi - Mitchell Zuckoff.epub",
		},
	}

	for _, c := range cases {
		got := Parse(c.in)
		if got.Title != c.Title {
			t.Errorf("%q\n  título:  got %q, want %q", c.in, got.Title, c.Title)
		}
		if got.Author != c.Author {
			t.Errorf("%q\n  autor:   got %q, want %q", c.in, got.Author, c.Author)
		}
		if got.Series != c.Series {
			t.Errorf("%q\n  série:   got %q, want %q", c.in, got.Series, c.Series)
		}
		if got.HasIdx != c.HasIdx || (c.HasIdx && got.Index != c.index) {
			t.Errorf("%q\n  índice:  got %v/%v, want %v/%v", c.in, got.Index, got.HasIdx, c.index, c.HasIdx)
		}
		if n := got.NewFilename(); n != c.newName {
			t.Errorf("%q\n  nome:    got %q\n           want %q", c.in, n, c.newName)
		}
	}
}

func TestTitleCasePT(t *testing.T) {
	cases := []struct{ in, want string }{
		{"666 - O LIMIAR DO INFERNO", "666 - O Limiar do Inferno"},
		{"A AMBICAO DE UM HOMEM", "A Ambicao de um Homem"},
		{"A ARTE DA NAO CONFORMIDADE", "A Arte da Nao Conformidade"},
		{"A ESTIRPE DO DRAGAO", "A Estirpe do Dragao"},
		{"A CARNE DE DEUS", "A Carne de Deus"},
	}
	for _, c := range cases {
		if !IsShouty(c.in) {
			t.Errorf("IsShouty(%q) = false, esperava true", c.in)
			continue
		}
		if got := TitleCasePT(c.in); got != c.want {
			t.Errorf("TitleCasePT(%q)\n  got  %q\n  want %q", c.in, got, c.want)
		}
	}
	// Títulos normais não devem ser reconhecidos como gritados.
	for _, s := range []string{"1984", "A culpa é das estrelas", "O Silmarillion", "EUA hoje"} {
		if IsShouty(s) {
			t.Errorf("IsShouty(%q) = true, não deveria mexer neste título", s)
		}
	}
}

func TestRestoreColon(t *testing.T) {
	cases := []struct{ in, want string }{
		{"13 Horas_ Os Soldados Secretos", "13 Horas: Os Soldados Secretos"},
		{"10_ mais feliz", "10: mais feliz"},
		{"A garota do calendario_ Dezembro", "A garota do calendario: Dezembro"},
		{"snake_case_intacto", "snake_case_intacto"}, // sem espaço depois: não mexe
	}
	for _, c := range cases {
		if got := RestoreColon(c.in); got != c.want {
			t.Errorf("RestoreColon(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAuthorFileAs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"J. R. R. Tolkien", "Tolkien, J. R. R."},
		{"George Orwell", "Orwell, George"},
		{"Alberto da Costa e Silva", "Alberto da Costa e Silva"}, // "e" indica multi-autor
		{"Gabriel Garcia Marquez", "Marquez, Gabriel Garcia"},
		{"Machado de Assis", "de Assis, Machado"},
		{"Tolkien, J. R. R.", "Tolkien, J. R. R."}, // já invertido
		{"Platao", "Platao"},                       // nome único
	}
	for _, c := range cases {
		if got := AuthorFileAs(c.in); got != c.want {
			t.Errorf("AuthorFileAs(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// O dc:Title interno não contém autor: um " - " ali é parte do título. Este teste
// trava a regressão em que "666 - O LIMIAR DO INFERNO" virava só "666".
func TestParseTitleOnlyNaoCortaAutor(t *testing.T) {
	cases := []struct{ in, want string }{
		{"666 - O LIMIAR DO INFERNO", "666 - O LIMIAR DO INFERNO"},
		{"A Batalha de Kadesh - Ramses - Vol 3", "A Batalha de Kadesh"},
		{"[Maze Runner 2]Prova de fogo", "Prova de fogo"},
		{"A Baronesa(Oficial)", "A Baronesa"},
		{"Revista Trasgo - Edicao 04", "Revista Trasgo - Edicao 04"},
	}
	for _, c := range cases {
		if got := ParseTitleOnly(c.in).Title; got != c.want {
			t.Errorf("ParseTitleOnly(%q).Title = %q, want %q", c.in, got, c.want)
		}
		if a := ParseTitleOnly(c.in).Author; a != "" {
			t.Errorf("ParseTitleOnly(%q) extraiu autor %q, não deveria", c.in, a)
		}
	}
}

// Trava a regressão do par "2° chance" / "2º chance": símbolo de grau e ordinal
// masculino são caracteres distintos que passam por unicode.IsLetter, e precisam ser
// descartados para as duas grafias do mesmo livro colidirem na mesma chave.
func TestNormKeyDescartaNaoASCII(t *testing.T) {
	if a, b := NormKey("2° chance"), NormKey("2º chance"); a != b {
		t.Errorf("NormKey divergiu: %q vs %q", a, b)
	}
	if a, b := NormKey("A Ameaça"), NormKey("A AMEACA"); a != b {
		t.Errorf("acento/caixa deveriam colidir: %q vs %q", a, b)
	}
	if got := NormKey("1ª Edição — Vol. 2"); got != "1 edicao vol 2" {
		t.Errorf("NormKey(%q) = %q", "1ª Edição — Vol. 2", got)
	}
}
