package target

import (
	"github.com/isaacdmcampos/robooks/internal/epub"
	"github.com/isaacdmcampos/robooks/internal/meta"
)

// Kavita aplica as convenções do Kavita para bibliotecas de livros.
//
// Duas regras vêm da documentação e uma custou uma biblioteca inteira para descobrir:
//
//   - Nenhum arquivo pode ficar na raiz da biblioteca. Com os arquivos soltos, o Kavita
//     trata a raiz como se fosse uma pasta de série, registra
//     "consists of one or more Series folders as a library root, using series scan" e
//     passa a varrer apenas séries já conhecidas — numa biblioteca recém-criada isso
//     significa zero livros indexados, com o scan terminando em 4 ms.
//
//   - Cada livro precisa de uma pasta. Séries compartilham a pasta da série; avulsos
//     ganham pasta própria.
//
//   - O agrupamento vem de calibre:series no OPF, não do nome da pasta: para epub, o
//     Kavita não usa a hierarquia de diretórios ("eBooks do not fall back to folders
//     for parsing"). A pasta é só organização de disco.
type Kavita struct{}

func (Kavita) Name() string { return "kavita" }

func (Kavita) Describe() string {
	return "pasta por série (ou por título, para avulsos); série vem de calibre:series no OPF"
}

func (k Kavita) Place(b meta.Book, m epub.Meta) Placement {
	want := desiredMeta(b, m)

	// A pasta usa o nome ASCII, igual ao nome do arquivo: acentos vão para o metadado,
	// que é o que o Kavita exibe, e o disco fica seguro em Samba/Windows.
	// O título do metadado vem antes do nome do arquivo: nomes de download costumam
	// carregar lixo ("1984 baixado da internet"), enquanto o dc:title é o do editor.
	dir := ""
	switch {
	case want.Series != "":
		dir = safeDir(want.Series)
	case b.Series != "":
		dir = safeDir(b.Series)
	case want.Title != "":
		dir = safeDir(want.Title)
	default:
		dir = safeDir(b.Title)
	}
	if dir == "" {
		dir = "_sem-titulo"
	}

	return Placement{
		Dir:      dir,
		Filename: b.NewFilename(),
		MetaArgs: metaArgsFor(want, m),
	}
}
