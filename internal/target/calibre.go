package target

import (
	"path/filepath"
	"strings"

	"github.com/isaacdmcampos/kinava/internal/epub"
	"github.com/isaacdmcampos/kinava/internal/meta"
)

// Calibre aplica o layout "Autor/Título", que é o padrão de fato do Calibre e o que a
// maioria das ferramentas de ebook entende ao importar uma pasta.
//
// Difere do Kavita em dois pontos que importam: o Calibre agrupa por autor no primeiro
// nível (o Kavita agrupa por série), e usa o nome da pasta como identidade do livro,
// então volumes de uma série ficam em pastas separadas em vez de compartilhadas.
type Calibre struct{}

func (Calibre) Name() string { return "calibre" }

func (Calibre) Describe() string {
	return "Autor/Título/ — primeiro nível por autor, uma pasta por livro"
}

func (c Calibre) Place(b meta.Book, m epub.Meta) Placement {
	want := desiredMeta(b, m)

	author := want.Author
	if author == "" {
		author = b.Author
	}
	if author == "" {
		author = "Autor Desconhecido"
	}
	// O Calibre lista por sobrenome; usar a forma ordenável no diretório faz a pasta
	// aparecer no mesmo lugar em que a interface espera o autor.
	if s := meta.AuthorFileAs(author); s != "" {
		author = s
	}

	title := want.Title
	if title == "" {
		title = b.Title
	}
	// Volume no começo do nome da pasta mantém a ordem de leitura na listagem do disco.
	if want.Series != "" && want.SeriesIndex > 0 {
		title = meta.Collapse(want.Series + " " + meta.FmtIndex(want.SeriesIndex) + " - " + title)
	}

	dir := filepath.Join(safeDir(author), safeDir(title))
	if strings.TrimSpace(dir) == "" {
		dir = "_sem-titulo"
	}
	return Placement{
		Dir:      dir,
		Filename: b.NewFilename(),
		MetaArgs: metaArgsFor(want, m),
	}
}
