package target

import (
	"github.com/isaacmirandacampos/robooks/internal/epub"
	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// Grimmory aplica as convenções do Grimmory.
//
// O layout é o mesmo do Kavita — pasta por série, ou por título para avulsos — e isso
// não é coincidência: os dois leem os metadados de dentro do epub e usam a pasta apenas
// como organização de disco. A diferença que importa está na configuração da biblioteca,
// não no disco: o Grimmory precisa do modo "one book per file". Com "one book per
// folder" ele leria cada pasta de série como um único livro, e nesta biblioteca isso
// esconderia 393 volumes distribuídos por 212 pastas.
//
// O alvo existe separado do Kavita mesmo com o layout igual porque as convenções podem
// divergir a qualquer release, e descobrir isso com 11 mil arquivos no disco é caro.
type Grimmory struct{}

func (Grimmory) Name() string { return "grimmory" }

func (Grimmory) Describe() string {
	return "pasta por série (ou por título); exige a biblioteca em modo \"one book per file\""
}

func (g Grimmory) Place(b meta.Book, m epub.Meta) Placement {
	want := desiredMeta(b, m)

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
