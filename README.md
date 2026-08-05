# robooks

CLI agnóstico de ingestão de ebooks: detecta duplicata pelo conteúdo, normaliza
metadados e escreve no layout que o servidor de destino espera. Hoje suporta **Kavita** e
**Calibre**; adicionar outro alvo é implementar um método.

CLI para preparar ebooks baixados antes de entrarem numa biblioteca já organizada:
detecta duplicata pelo conteúdo, normaliza metadados e escreve no layout que o servidor
de destino espera.

```bash
go build -o robooks ./cmd/robooks

./robooks index                        # indexa a biblioteca (uma vez, depois é incremental)
./robooks ingest ~/Downloads/livros    # mostra o que faria
./robooks ingest -apply ~/Downloads/livros
./robooks check livro.epub             # "isto já existe?" — exit 1 se sim
./robooks targets                      # alvos suportados
```

Nada é escrito sem `-apply`.

## Por que detectar duplicata pelo conteúdo

Foi a única abordagem que funcionou numa biblioteca real de 11 mil livros. As anteriores
falharam assim:

| abordagem | por que falha |
|---|---|
| hash do arquivo | cada conversão do calibre grava um UUID novo; cópias do mesmo livro diferem em ~7 bytes |
| nome do arquivo normalizado | `3 Grau - Clube das Mulheres Contra o Crime` e `3.º Grau` são o mesmo livro |
| `dc:title` + `dc:creator` | não resolve typo (`A Arte de oOuvir o Coração`) nem subtítulo extra |
| hash exato do texto | zero grupos — o HTML gerado difere entre conversões |
| similaridade de título | `Dom Quixote parte I` e `parte II` são 98% parecidos e livros distintos |

O que resta é comparar o texto: semelhança de Jaccard sobre shingles de 8 palavras,
amostrados pelo **hash do próprio shingle**, nunca pela posição. Amostrar por conteúdo é
o detalhe que faz funcionar — um dos arquivos costuma ter um preâmbulo a mais e qualquer
amostragem posicional desalinha os textos.

Medido na biblioteca de referência:

```
mesmo livro, arquivos diferentes    96,5% – 100%
Dom Quixote parte I × parte II       1,1%
Pesadelos vol I × vol II             1,0%
```

O limiar padrão é 85%, com margem larga dos dois lados. Ajuste com `-similarity`.

## O índice

Calcular a assinatura de 11 mil livros leva ~1min14 com dez workers. Fazer isso a cada
ingestão de dois arquivos seria absurdo, então `robooks index` guarda as assinaturas em
`~/.cache/robooks/index.gz` e recalcula apenas o que mudou (por tamanho e mtime).

Fica fora da biblioteca de propósito: o Kavita varre a pasta e um arquivo estranho lá
dentro só geraria ruído no scan.

## Alvos

A interface `target.Target` isola o que cada servidor espera. Não é preferência estética:
escrever o layout errado quebra de verdade — com arquivos soltos na raiz, o Kavita entra
em modo "series scan" e para de indexar a biblioteca.

| alvo | layout | agrupamento |
|---|---|---|
| `kavita` | `Série/` ou `Título/` | `calibre:series` no OPF; pastas são ignoradas para epub |
| `calibre` | `Autor/Título/` | pasta é a identidade do livro |

Para adicionar um alvo, implemente `Place()` e registre em `target.Registry()`.

## Metadados

Aplicados a todo livro que entra, iguais para qualquer alvo:

- **`dc:title`** — prefere o título interno (costuma ter acentuação que o nome do arquivo
  perdeu), limpo de `[Série N]` e `(Oficial)`, com `:` restaurado no lugar de `_` e caixa
  normalizada quando o título está todo em maiúsculas.
- **`calibre:series` / `series_index`** — extraídos do nome quando o OPF não traz.
- **`opf:file-as`** — `Sobrenome, Nome`, corrigindo os `Unknown` que vêm de conversão de
  MOBI e atrapalham a ordenação por autor.

Nomes de arquivo e de pasta ficam em ASCII; os acentos vão para o metadado, que é o que a
interface exibe. Isso mantém o disco seguro em compartilhamento Samba/Windows.

## Conversão

`.mobi`, `.azw3` e `.azw` são convertidos via calibre com `--flow-size 0`. A opção desliga
a divisão do HTML por tamanho; sem ela, livros sem ponto de quebra abaixo do limite
abortam com `Could not find reasonable point at which to split` e não geram saída. Não
altera texto nem imagens.

O calibre é invocado como `/usr/bin/python3 /usr/bin/ebook-convert`, com ambiente mínimo.
O shebang dele é `#!/usr/bin/env python3`, e gerenciadores de versão (mise, pyenv, conda)
colocam à frente um Python sem os módulos do calibre — o script morre com
`ModuleNotFoundError`. Invocar o interpretador do sistema elimina a dependência do PATH.

## Flags principais

| flag | padrão | |
|---|---|---|
| `-lib` | `~/kavita/data/Livros` | raiz da biblioteca |
| `-target` | `kavita` | alvo de layout |
| `-apply` | | sem isso, apenas relata |
| `-similarity` | `0.85` | limiar de duplicata |
| `-on-duplicate` | `best` | `best`, `skip`, `quarantine`, `replace` |
| `-convert` | `true` | converter mobi/azw3 |
| `-enrich` | `false` | consultar fontes externas (gêneros, ISBN, editora, sinopse) |
| `-tags-pt` | `true` | traduzir os gêneros para português |
| `-workers` | `NumCPU-2` | paralelismo |

## Duplicata: qual cópia fica

O padrão é `best` — quando o arquivo baixado é o mesmo livro que já está na biblioteca,
fica o melhor dos dois e o outro vai para a quarentena. A ordem de decisão é:

1. **Série no metadado.** É o que faz o Kavita agrupar o volume. Uma cópia com
   `calibre:series` vale mais que uma maior sem série — "Mago Negro 02 - A Aprendiz"
   contra "A Aprendiz" solto.
2. **Mais texto.** Edições divergem em conteúdo real: "Nove Semanas e Meia de Amor" tem
   44508 palavras contra 34531 da outra cópia do mesmo livro.
3. **Mais bytes.** Só como desempate — mesmo texto e mesma série, o arquivo maior
   costuma ter imagens melhores.

O motivo de bytes ficar por último é que o tamanho sobe com imagens e enganaria a
escolha, descartando a versão textualmente mais completa.

O dry-run sempre diz **por quê**:

```
SUBSTITUI  A Fada - Carolina Munhoz.epub
           100% igual a A Fada/A Fada - Carolina Munhoz.epub
           o novo tem série no metadado
```

Nada é apagado: a cópia perdedora vai para `_duplicatas`.

## Metadados externos (`-enrich`)

Completa o que o arquivo não traz — gêneros, ISBN, editora, sinopse — via
`fetch-ebook-metadata` do calibre, que agrega várias fontes.

Só preenche lacuna, nunca sobrescreve: o metadado do arquivo veio da editora ou da
conversão e costuma estar certo, enquanto a consulta externa é um palpite baseado em
título e autor.

Os gêneros voltam em inglês mesmo para livros em português; `-tags-pt` (ligado por
padrão) traduz os mais comuns:

```
Fiction, Classics, Fantasy, Epic  ->  Ficção, Clássicos, Fantasia, Épico
```

Custa **7–25 s por livro**, e é por isso que está desligado por padrão e faz sentido no
ingest (poucos arquivos) e não em lote — para 11 mil livros passaria de dez horas.

Duas alternativas foram testadas e descartadas: a API do Google Books devolve
`Quota exceeded` sem chave, e o Open Library não acha títulos em português ("O
Silmarillion" retorna zero, "The Silmarillion" retorna 37).

## Instalação

```bash
GOBIN=$HOME/.local/bin go install ./cmd/robooks
```

Evite o `go install` sem `GOBIN`: com o Go gerenciado por mise/asdf, o binário vai parar
dentro da pasta da versão do Go e some na próxima atualização.

## Estado

Funciona ponta a ponta: indexação incremental, dedup por conteúdo, escolha da melhor
cópia, metadados locais e externos, layout para Kavita e Calibre, dry-run em tudo.
