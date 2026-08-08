package genre

import (
	"regexp"
	"sort"
	"strings"

	"github.com/isaacmirandacampos/robooks/internal/meta"
)

// Este arquivo é a variante em inglês da canonização.
//
// O acervo passou a receber metadados de APIs que respondem em inglês, e traduzir de
// volta para português criava um vocabulário híbrido: "Fiction" e "Ficção" viravam dois
// filtros para a mesma coisa, agora com o agravante de que a próxima importação
// recriaria o rótulo em inglês. Padronizar no idioma da fonte encerra o vaivém.
//
// O mapa abaixo foi construído a partir do vocabulário real de uma biblioteca de 11 mil
// livros — 1786 categorias distintas, das quais 660 apareciam uma única vez.

// canonEN unifica variantes numa forma só, em inglês. O valor vazio significa descarte:
// o rótulo existe, mas não descreve o assunto do livro.
var canonEN = map[string]string{
	// --- núcleo ---
	"fiction": "Fiction", "ficcao": "Fiction", "ficção": "Fiction",
	"general fiction": "Fiction", "genre fiction": "Fiction", "novels": "Fiction",
	"novel": "Fiction", "romance ficcao": "Fiction",
	"nonfiction": "Nonfiction", "non-fiction": "Nonfiction", "non fiction": "Nonfiction",
	"nao-ficcao": "Nonfiction", "não-ficção": "Nonfiction", "naoficcao": "Nonfiction",

	"fantasy": "Fantasy", "fantasia": "Fantasy", "fantasy fiction": "Fantasy",
	"high fantasy": "High Fantasy", "urban fantasy": "Urban Fantasy",
	"epic fantasy": "Epic Fantasy", "dark fantasy": "Dark Fantasy",
	"historical fantasy": "Historical Fantasy", "fantasy romance": "Fantasy Romance",

	"science fiction": "Science Fiction", "sci-fi": "Science Fiction",
	"scifi": "Science Fiction", "sci fi": "Science Fiction",
	"ficcao cientifica": "Science Fiction", "ficção científica": "Science Fiction",
	"science fiction fantasy": "Science Fiction", "science fiction & fantasy": "Science Fiction",
	"cyberpunk": "Cyberpunk", "steampunk": "Steampunk", "space opera": "Space Opera",

	"horror": "Horror", "terror": "Horror", "gothic horror": "Gothic",
	"gothic": "Gothic", "lovecraftian": "Lovecraftian", "horror thriller": "Horror",

	"thriller": "Thriller", "thrillers": "Thriller", "suspense": "Thriller",
	"thriller & suspense": "Thriller", "psychological thriller": "Psychological Thriller",
	"legal thriller": "Legal Thriller", "political thriller": "Political Thriller",
	"techno-thriller": "Thriller",

	"mystery": "Mystery", "misterio": "Mystery", "mistério": "Mystery",
	"mystery thriller": "Mystery", "murder mystery": "Mystery",
	"mystery & detective": "Mystery", "thriller & mystery": "Mystery",
	"historical mystery": "Historical Mystery", "cozy mystery": "Cozy Mystery",

	"crime": "Crime", "true crime": "True Crime", "crime fiction": "Crime",
	"detective": "Detective", "detetive": "Detective", "policial": "Detective",
	"police procedural": "Detective", "police": "Detective", "murder": "Crime",
	"serial killer": "True Crime", "noir": "Noir", "espionage": "Espionage",
	"espionagem": "Espionage", "spy": "Espionage", "spy thriller": "Espionage",

	"romance": "Romance", "romances": "Romance", "love stories": "Romance",
	"romantico": "Romance", "romântico": "Romance",
	"contemporary romance": "Contemporary Romance", "historical romance": "Historical Romance",
	"romance histórico": "Historical Romance", "romance historico": "Historical Romance",
	"paranormal romance": "Paranormal Romance", "regency": "Regency Romance",
	"regency romance": "Regency Romance", "rom com": "Romantic Comedy",
	"romantic comedy": "Romantic Comedy", "chick lit": "Chick Lit", "chick-lit": "Chick Lit",
	"chicklit": "Chick Lit", "erotica": "Erotica", "erotico": "Erotica",
	"erótico": "Erotica", "smut": "Erotica", "spicy": "Erotica",

	"historical fiction": "Historical Fiction", "historical": "Historical Fiction",
	"histórico": "Historical Fiction", "historico": "Historical Fiction",
	"ficção histórica": "Historical Fiction", "ficcao historica": "Historical Fiction",
	"history": "History", "historia": "History", "história": "History",
	"world history": "History", "ancient history": "Ancient History",
	"military history": "Military History", "american history": "History",
	"european history": "History", "war": "War", "guerra": "War",
	"military": "Military", "military fiction": "Military Fiction",
	"holocaust": "Holocaust", "wwii": "War", "world war ii": "War",

	"biography": "Biography", "biografia": "Biography",
	"biography & autobiography": "Biography", "biography memoir": "Biography",
	"biographies & memoirs": "Biography", "autobiography": "Biography",
	"autobiografia": "Biography", "memoir": "Memoir", "memoirs": "Memoir",
	"memórias": "Memoir", "memorias": "Memoir", "personal memoirs": "Memoir",

	"young adult": "Young Adult", "young adult fiction": "Young Adult", "ya": "Young Adult",
	"teen & young adult": "Young Adult", "teen": "Young Adult",
	"young adult contemporary": "Young Adult", "young adult romance": "Young Adult",
	"young adult paranormal": "Young Adult", "young adult fantasy": "Young Adult",
	"middle grade": "Middle Grade",
	"childrens":    "Children's", "children": "Children's", "children's": "Children's",
	"childrens books": "Children's", "juvenile fiction": "Children's",
	"juvenile": "Children's", "juvenile nonfiction": "Children's",
	"infantojuvenil": "Young Adult", "infanto-juvenil": "Young Adult",
	"infantil": "Children's", "fairy tales": "Fairy Tales",

	"classics": "Classics", "classicos": "Classics", "clássicos": "Classics",
	"classico": "Classics", "clássico": "Classics", "classic literature": "Classics",
	"literature": "Literature", "literatura": "Literature", "literary": "Literature",
	"literature & fiction": "Literature", "literary fiction": "Literary Fiction",
	"literary criticism": "Literary Criticism", "criticism": "Literary Criticism",
	"critica literaria": "Literary Criticism", "crítica literária": "Literary Criticism",
	"literary collections": "Anthologies", "anthologies": "Anthologies",
	"collections": "Anthologies", "coletanea": "Anthologies",

	"poetry": "Poetry", "poesia": "Poetry", "poemas": "Poetry", "poema": "Poetry",
	"drama": "Drama", "teatro": "Theatre", "theatre": "Theatre", "theater": "Theatre",
	"plays": "Theatre", "short stories": "Short Stories", "contos": "Short Stories",
	"conto": "Short Stories", "short stories (single author)": "Short Stories",
	"essays": "Essays", "ensaios": "Essays", "crônicas": "Essays", "cronicas": "Essays",

	"adventure": "Adventure", "aventura": "Adventure", "aventure": "Adventure",
	"adventure stories": "Adventure", "action": "Action", "acao": "Action", "ação": "Action",
	"action & adventure": "Adventure", "ação e aventura": "Adventure",
	"viagem e aventura": "Adventure", "westerns": "Western", "western": "Western",
	"nautical": "Nautical", "survival": "Survival",

	"dystopian": "Dystopia", "dystopia": "Dystopia", "distopia": "Dystopia",
	"utopian": "Utopia", "post apocalyptic": "Post-Apocalyptic",
	"post-apocalyptic": "Post-Apocalyptic", "apocalyptic": "Post-Apocalyptic",

	"paranormal": "Paranormal", "supernatural": "Supernatural",
	"sobrenatural": "Supernatural", "magic": "Magic", "magia": "Magic",
	"witches": "Witches", "vampires": "Vampires", "werewolves": "Werewolves",
	"dragons": "Dragons", "demons": "Demons", "angels": "Angels", "ghosts": "Ghosts",
	"mythology": "Mythology", "mitologia": "Mythology", "greek mythology": "Mythology",
	"norse mythology": "Mythology", "superheroes": "Superheroes",

	// --- não-ficção ---
	"philosophy": "Philosophy", "filosofia": "Philosophy",
	"epistemology": "Philosophy", "ethics": "Philosophy", "metaphysics": "Philosophy",
	"psychology": "Psychology", "psicologia": "Psychology",
	"psicanalise": "Psychoanalysis", "psicanálise": "Psychoanalysis",
	"psychoanalysis": "Psychoanalysis", "neuroscience": "Neuroscience",
	"mental illness": "Mental Health", "mental health": "Mental Health",
	"sociology": "Sociology", "sociologia": "Sociology",
	"social science": "Social Sciences", "ciencias sociais": "Social Sciences",
	"ciências sociais": "Social Sciences", "social sciences": "Social Sciences",
	"anthropology": "Anthropology", "antropologia": "Anthropology",
	"politics": "Politics", "politica": "Politics", "política": "Politics",
	"political science": "Politics", "ciencia politica": "Politics",
	"ciência política": "Politics", "politics & social sciences": "Politics",
	"economics": "Economics", "economia": "Economics",
	"business & economics": "Business", "business": "Business", "negocios": "Business",
	"negócios": "Business", "administração": "Management", "administracao": "Management",
	"management": "Management", "leadership": "Leadership", "lideranca": "Leadership",
	"liderança": "Leadership", "entrepreneurship": "Entrepreneurship",
	"finance": "Finance", "financas": "Finance", "finanças": "Finance",
	"money": "Finance", "investing": "Finance", "marketing": "Marketing",

	"science": "Science", "ciencia": "Science", "ciência": "Science",
	"popular science": "Science", "physics": "Physics", "fisica": "Physics",
	"física": "Physics", "biology": "Biology", "biologia": "Biology",
	"chemistry": "Chemistry", "quimica": "Chemistry", "química": "Chemistry",
	"mathematics": "Mathematics", "matematica": "Mathematics", "matemática": "Mathematics",
	"astronomy": "Astronomy", "astronomia": "Astronomy", "nature": "Nature",
	"environment": "Environment", "evolution": "Science",
	"technology": "Technology", "tecnologia": "Technology",
	"computers": "Computing", "computing": "Computing",
	"computer science": "Computing", "programming": "Programming",
	"artificial intelligence": "Artificial Intelligence",

	"religion": "Religion", "religiao": "Religion", "religião": "Religion",
	"christianity": "Christianity", "cristianismo": "Christianity",
	"christian living": "Christianity", "christian non fiction": "Christianity",
	"catholic": "Christianity", "catolicismo": "Christianity", "theology": "Theology",
	"teologia": "Theology", "bible": "Religion", "biblia": "Religion",
	"islam": "Islam", "judaism": "Judaism", "jewish": "Judaism",
	"buddhism": "Buddhism", "budismo": "Buddhism", "hinduism": "Hinduism",
	"atheism": "Atheism", "ateismo": "Atheism", "ateísmo": "Atheism",
	"spirituality": "Spirituality", "espiritualidade": "Spirituality",
	"espiritual": "Spirituality", "mind & spirit": "Spirituality",
	"religion & spirituality": "Religion", "new age": "New Age",
	"occultism": "Occult", "occult": "Occult", "ocultismo": "Occult",
	"mysticism": "Mysticism", "esoterismo": "Esotericism",
	// O acervo tem uma coleção espírita grande; sem estas linhas ela se espalha por
	// vinte rótulos diferentes.
	"espiritismo": "Spiritism", "espiritualismo": "Spiritism", "espírita": "Spiritism",
	"espirita": "Spiritism", "livro espírita": "Spiritism", "doutrina espírita": "Spiritism",
	"codificação espírita": "Spiritism", "mediunidade": "Spiritism",
	"reencarnação": "Spiritism", "reencarnacao": "Spiritism",
	"vida após a morte": "Spiritism", "imortalidade": "Spiritism",

	"self-help": "Self-Help", "self help": "Self-Help", "selfhelp": "Self-Help",
	"autoajuda": "Self-Help", "auto-ajuda": "Self-Help",
	"personal growth": "Self-Help", "personal development & self-help": "Self-Help",
	"motivational & inspirational": "Self-Help", "motivational": "Self-Help",
	"inspirational": "Self-Help", "how to": "Self-Help",
	"professional development": "Self-Help", "productivity": "Productivity",
	"relationships": "Relationships", "marriage": "Relationships",
	"parenting": "Parenting", "family": "Family", "familia": "Family", "família": "Family",

	"health": "Health", "saude": "Health", "saúde": "Health",
	"health & fitness": "Health", "fitness": "Fitness", "medical": "Medicine",
	"medicina": "Medicine", "medicine": "Medicine", "nutrition": "Nutrition",
	"cooking": "Cooking", "culinaria": "Cooking", "culinária": "Cooking",
	"cookbooks": "Cooking", "gastronomia": "Cooking", "food": "Cooking",
	"sports": "Sports", "esportes": "Sports", "soccer": "Sports", "football": "Sports",

	"art": "Art", "arte": "Art", "music": "Music", "musica": "Music", "música": "Music",
	"rock n roll": "Music", "film": "Film", "cinema": "Film", "photography": "Photography",
	"fotografia": "Photography", "architecture": "Architecture",
	"design": "Design", "comics": "Comics", "quadrinhos": "Comics",
	"comics & graphic novels": "Comics", "graphic novels": "Graphic Novel",
	"graphic novel": "Graphic Novel", "manga": "Manga", "mangá": "Manga",
	"video games": "Games", "games": "Games", "gaming": "Games",

	"education": "Education", "educacao": "Education", "educação": "Education",
	"teaching": "Education", "writing": "Writing", "escrita": "Writing",
	"language arts & disciplines": "Language", "linguistics": "Language",
	"linguistica": "Language", "linguística": "Language",
	"law": "Law", "direito": "Law", "travel": "Travel", "viagem": "Travel",
	"humor": "Humor", "comedy": "Humor", "comédia": "Humor", "comedia": "Humor",
	"satire": "Satire", "satira": "Satire", "sátira": "Satire",

	"lgbtq": "LGBTQ", "lgbt": "LGBTQ", "queer": "LGBTQ", "gay": "LGBTQ",
	"lesbian": "LGBTQ", "lesbians": "LGBTQ", "boy's love": "LGBTQ",
	"transgender": "LGBTQ", "intersex": "LGBTQ", "gender": "Gender",
	"feminism": "Feminism", "feminismo": "Feminism", "race": "Race",
	"african american": "African American", "social justice": "Social Justice",

	// --- literaturas nacionais: filtro legítimo num acervo multilíngue ---
	"portuguese literature":      "Portuguese Literature",
	"literatura portuguesa":      "Portuguese Literature",
	"brazilian literature":       "Brazilian Literature",
	"literatura brasileira":      "Brazilian Literature",
	"brazilian fiction":          "Brazilian Literature",
	"british literature":         "British Literature",
	"english literature":         "British Literature",
	"literatura inglesa":         "British Literature",
	"irish literature":           "Irish Literature",
	"american literature":        "American Literature",
	"literatura norte-americana": "American Literature",
	"americana":                  "American Literature",
	"french literature":          "French Literature",
	"literatura francesa":        "French Literature",
	"russian literature":         "Russian Literature",
	"literatura russa":           "Russian Literature",
	"japanese literature":        "Japanese Literature",
	"asian literature":           "Asian Literature",
	"latin american literature":  "Latin American Literature",
	"spanish literature":         "Spanish Literature",
	"german literature":          "German Literature",
	"italian literature":         "Italian Literature",
	"polish literature":          "Polish Literature",
	"scandinavian literature":    "Scandinavian Literature",
	"african literature":         "African Literature",
	"world literature":           "World Literature",

	// --- descartes explícitos ---
	// Formato do arquivo, não assunto. "Audiobook" sozinho marcava 1352 livros e não
	// respondia nada sobre nenhum deles.
	"audiobook": "", "audio book": "", "audiobooks": "", "ebooks": "", "ebook": "",
	"e-book": "", "large type books": "", "paperback": "", "hardcover": "",
	"kindle": "", "unfinished": "", "audiolivro": "",
	// Rótulos de loja e de clube de leitura.
	"book club": "", "books about books": "", "amazon": "", "goodreads": "",
	"bestseller": "", "best seller": "", "new york times": "", "oprah": "",
	// Vazios.
	"general": "", "geral": "", "adult": "", "adult fiction": "", "all": "",
	"misc": "", "miscellaneous": "", "other": "", "outros": "", "social": "",
	"life": "", "research": "", "research methods": "", "google": "",
	// Recorte geográfico puro: onde se passa não é o que o livro é.
	"brazil": "", "brasil": "", "russia": "", "asia": "", "europe": "", "africa": "",
	"france": "", "paris": "", "spain": "", "china": "", "japan": "", "india": "",
	"ireland": "", "canada": "", "great britain": "", "england": "", "scotland": "",
	"scottish": "", "germany": "", "italy": "", "portugal": "", "new york": "",
	"middle east": "", "latin america": "", "south america": "", "united states": "",
	"american": "", "european": "", "asian": "", "african": "", "irish": "",
	"christmas": "",
	// Período: útil para quem cataloga, ruído para quem escolhe o que ler.
	"20th century": "", "21st century": "", "19th century": "", "18th century": "",
	"ancient": "", "medieval": "", "victorian": "", "renaissance": "",
	// Idioma no lugar de gênero.
	"spanish": "", "french": "", "german": "", "italian": "", "portuguese": "",
	"english": "", "foreign language study": "", "german language materials": "",

	// --- segunda passada: o que sobrou depois de rodar sobre o acervo real ---
	// Variantes que escaparam por diferença de número ou de grafia.
	"short story": "Short Stories", "short story collection": "Short Stories",
	"short stories (single author": "Short Stories", "novella": "Novella",
	"novela": "Novella", "detective and mystery stories": "Detective",
	"mysterythriller": "Mystery", "mistério e suspense": "Mystery",
	"misterio e suspense": "Mystery", "investigação": "Detective",
	"crime investigations": "Crime", "hard boiled": "Noir", "nordic noir": "Noir",
	"alternative history": "Alternate History", "african americans": "African American",
	"diary": "Diaries", "humorous": "Humor", "humour": "Humor",
	"graphic novels comics": "Graphic Novel", "bande dessinée": "Comics",
	"diet": "Health", "diets": "Health", "militar": "Military",
	"segunda guerra mundial": "War", "não ficção": "Nonfiction", "nao ficcao": "Nonfiction",
	"infanto juvenil": "Young Adult", "jeune adulte": "Young Adult", "adulte": "",
	"new adult": "New Adult", "romantasy": "Fantasy Romance",
	"hard science fiction": "Science Fiction", "speculative fiction": "Science Fiction",
	"interplanetary voyages": "Science Fiction", "cthulhu mythos": "Lovecraftian",
	"horror tales": "Horror", "monsters": "Horror", "aliens": "Science Fiction",
	"shapeshifters": "Paranormal", "elfos": "Fantasy",
	"desenvolvimento pessoal": "Self-Help", "personal development": "Self-Help",
	"criatividade": "Creativity", "creativity": "Creativity",
	"jornalismo": "Journalism", "journalism": "Journalism",
	"governo e política": "Politics", "government": "Politics",
	"ideologias e doutrinas": "Politics", "nacionalismo e patriotismo": "Politics",
	"sociedade e ciências sociais": "Social Sciences", "society": "Social Sciences",
	"cultural studies": "Cultural Studies", "cultura": "Cultural Studies",
	"neurobiologia": "Neuroscience", "sexo": "Sexuality", "sexuality": "Sexuality",
	"drogas": "Drugs", "assassino": "Crime", "dor": "", "sofrimento": "",
	"oração": "Religion", "moral cristã": "Christianity", "doutrina": "Spiritism",
	"desdobramento": "Spiritism", "sonambulismo": "Spiritism", "passe": "Spiritism",
	"médium": "Spiritism", "medium": "Spiritism", "intercâmbio mediúnico": "Spiritism",
	"misticismo": "Mysticism",
	// A coleção espírita traz o vocabulário interno da doutrina como se fosse gênero.
	// São conceitos de dentro dos livros, não categorias de acervo: separados, viram
	// vinte filtros de um punhado de livros cada; juntos, viram um filtro que funciona.
	"expiação e prova": "Spiritism", "leis morais": "Spiritism", "caridade": "Spiritism",
	"causa e efeito": "Spiritism", "clarividência": "Spiritism", "fluidos": "Spiritism",
	"fonte viva": "Spiritism", "magnetismo": "Spiritism", "materialização": "Spiritism",
	"mentor espiritual": "Spiritism", "milagres": "Spiritism", "possessão": "Spiritism",
	"prece": "Spiritism", "provação": "Spiritism", "prática mediúnica": "Spiritism",
	"reforma íntima": "Spiritism", "umbral": "Spiritism", "consciência": "Spiritism",
	"federação espírita brasileira": "Spiritism", "logosofia": "Spiritism",
	"conselho espírita internacional": "Spiritism", "filosofia e religião": "Religion",
	"ensinamentos": "Spiritism", "esperança": "Spiritism", "vida": "Spiritism",
	// Typos e variantes de número que criavam filtros gêmeos.
	"buisness": "Business", "biographies": "Biography", "womens": "Women",
	"womens fiction": "Women's Fiction", "contemporaryromance": "Contemporary Romance",
	"youngadult": "Young Adult", "jovem adulto": "Young Adult",
	"suspense e mistério": "Mystery", "suspense fiction": "Thriller",
	"romance - ficção": "Romance", "vampiros": "Vampires", "fantasmas": "Ghosts",
	"faroeste": "Western", "épico": "Epic", "epico": "Epic", "ensaio": "Essays",
	"poesia brasileira": "Poetry", "poemas brasileiros": "Poetry",
	"poesia portuguesa": "Poetry", "contos de fadas": "Fairy Tales", "fábulas": "Fables",
	"literatura infantil": "Children's", "literatura infanto-juvenil": "Young Adult",
	"literatura juvenil": "Young Adult", "literatura infantil mundial": "Children's",
	"filosofia da mente": "Philosophy", "filosofia e ciências sociais": "Philosophy",
	"teoria literária": "Literary Criticism", "tradução": "Language",
	"comunicação": "Communication", "comportamento": "Psychology",
	"jurídico": "Law", "leis": "Law", "escravidão": "History", "corrupção": "Politics",
	"cosmologia": "Science", "processo saúde-doença": "Health", "suicídio": "Mental Health",
	"literatura estrangeira": "World Literature", "literatura internacional": "World Literature",
	"literatura nacional": "Brazilian Literature", "literatura americana": "American Literature",
	"literatura latino americana": "Latin American Literature",
	"literatura fantstica":        "Fantasy", "portugues": "", "roman": "", "classique": "",
	"philosophie": "",
	// Estante pessoal e resto de catalogação de loja.
	"to-read": "", "read for school": "", "bookclub": "", "booktok": "",
	"vaga-lume": "", "coleção vaga-lume": "", "série vaga-lume": "", "vaquinha": "",
	"classicos todolivro": "", "belli studio": "", "revisados nacionais": "",
	"volume 1": "", "volume 2": "", "livro 2": "", "rayel": "", "leitura": "",
	"large print books": "", "media tie in": "", "media tie-in": "",
	"relato": "", "reportagem": "", "revista": "", "entrevistas": "", "cartas": "",
	"curiosidades": "", "estados unidos": "", "the world": "", "urban": "",
	"stem": "", "technical": "", "textbooks": "", "profissional e técnico": "",
	// Resto de pipeline de conversão e de loja, não assunto.
	"digital source": "", "digital source http": "", "amostra de livro": "",
	"novo conceito": "", "adult trade": "", "artigos": "", "diálogo": "",
	"oab": "", "dallas": "", "modern": "", "southern": "", "cultural": "",
	"sagas": "", "holiday": "", "sad": "", "dark": "", "space": "", "mind": "",
	"spirit": "", "emotions": "", "death": "", "grief": "",
	"academic": "", "academia": "", "humanities": "", "disciplines": "",
	"social themes": "", "banned books": "", "hugo awards": "", "nobel prize": "",
	"english fiction": "", "american fiction": "", "americans": "",
	"adventure and adventurers": "", "allegories": "",
}

var (
	// Século no lugar de gênero: "20th Century", "14th Century". Diz quando a história
	// se passa, não o que ela é.
	reSeculo = regexp.MustCompile(`(?i)^\d{1,2}(?:st|nd|rd|th)\s+century$|^s[ée]culo\s+[ivxl\d]+$`)
	// País ou região. A lista cobre o que apareceu no acervo; o que escapar cai no
	// filtro de frequência.
	paises = map[string]bool{
		"angola": true, "algeria": true, "australia": true, "denmark": true,
		"egypt": true, "greece": true, "iran": true, "iraq": true, "israel": true,
		"mozambique": true, "nigeria": true, "south africa": true, "soviet union": true,
		"lebanon": true, "turkey": true, "mexico": true, "argentina": true, "chile": true,
		"cuba": true, "korea": true, "vietnam": true, "afghanistan": true, "poland": true,
		"sweden": true, "norway": true, "finland": true, "iceland": true, "netherlands": true,
		"belgium": true, "austria": true, "switzerland": true, "hungary": true,
		"czech republic": true, "romania": true, "bulgaria": true, "ukraine": true,
		"inglaterra": true, "oriente médio": true, "oriente medio": true,
		"london": true, "berlin": true, "rome": true, "moscow": true, "tokyo": true,
		"california": true, "texas": true, "florida": true, "chicago": true, "boston": true,
	}
)

var (
	// Cabeçalho de assunto de biblioteca: "Los Angeles (calif.)", "Claire (fictitious
	// Character)", "Dune (imaginary Place)". Descreve a obra para um catalogador, não
	// para um leitor escolhendo o que ler.
	// O parêntese de fechamento é opcional porque a limpeza apara pontuação das pontas
	// antes de canonizar: "Dune (imaginary Place)" chega aqui já sem o ")".
	reLCSH = regexp.MustCompile(`(?i)\((?:fictitious|imaginary|legendary|mythical)\s+\w+\)?|` +
		`\((?:calif|ga|n\.y|tex|fla|ill|mass|pa|ohio|mich|wash|colo|ariz|ore|conn)\.?\)?$|` +
		`\bfictitious character\b`)
	// Franquia ou obra específica no lugar de gênero: "Harry Potter", "Star Trek
	// Original Series", "Harlequin Blaze". São etiquetas de uma obra só.
	reFranquia = regexp.MustCompile(`(?i)^(harry potter|percy jackson|star trek|star wars|` +
		`warcraft|harlequin\b.*|jane austen|sherlock holmes|discworld|middle-earth|` +
		`doctor who|marvel|dc comics|dune|game of thrones|twilight|hunger games)$`)
)

// CanonEN devolve a forma canônica em inglês de um rótulo, ou "" se ele deve sair.
//
// O segundo retorno distingue "não conheço este rótulo" de "conheço e mandei descartar":
// quem chama precisa poder tratar o vocabulário desconhecido por frequência, em vez de
// jogá-lo fora só porque não está no mapa.
func CanonEN(t string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(t))
	if v, ok := canonEN[k]; ok {
		return v, true
	}
	// Sem acento também: "ficção" e "ficcao" chegam das duas formas.
	if v, ok := canonEN[strings.ToLower(meta.DeaccentStr(k))]; ok {
		return v, true
	}
	if reLCSH.MatchString(t) || reFranquia.MatchString(strings.TrimSpace(t)) ||
		reSeculo.MatchString(k) || paises[k] || paises[strings.ToLower(meta.DeaccentStr(k))] {
		return "", true
	}
	return titleCase(t), false
}

// IsPersonName diz se o rótulo é, na verdade, o nome de alguém.
//
// Autor não é gênero, mas escapa de toda regra estrutural: "Agatha Christie" tem duas
// palavras capitalizadas e nenhum sinal de ruído. A única forma barata de reconhecê-lo é
// perguntar ao próprio catálogo — se o mesmo texto existe como autor, é autor.
//
// A lista vem de fora justamente para o pacote não precisar conhecer o servidor.
func IsPersonName(t string, nomes map[string]bool) bool {
	return nomes[strings.ToLower(strings.TrimSpace(t))]
}

// CleanEN é o Clean com saída em inglês: mesma remoção de ruído, mesma abertura de
// hierarquias, vocabulário canônico em inglês.
func CleanEN(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range tags {
		for _, part := range reSplit.Split(raw, -1) {
			t := strings.TrimSpace(part)
			t = strings.Trim(t, ".,;:-–—\"'()[]")
			t = meta.Collapse(t)
			if !usable(t) {
				continue
			}
			c, _ := CanonEN(t)
			if c == "" {
				continue
			}
			k := strings.ToLower(c)
			if seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}
