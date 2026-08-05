# kinava

CLI para preparar ebooks baixados antes de entrarem numa biblioteca já organizada:
detecta duplicata pelo conteúdo, normaliza metadados e escreve no layout que o servidor
de destino espera.

```bash
go build -o kinava ./cmd/kinava

./kinava index                        # indexa a biblioteca (uma vez, depois é incremental)
./kinava ingest ~/Downloads/livros    # mostra o que faria
./kinava ingest -apply ~/Downloads/livros
./kinava check livro.epub             # "isto já existe?" — exit 1 se sim
./kinava targets                      # alvos suportados
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
ingestão de dois arquivos seria absurdo, então `kinava index` guarda as assinaturas em
`~/.cache/kinava/index.gz` e recalcula apenas o que mudou (por tamanho e mtime).

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
| `-on-duplicate` | `skip` | `skip`, `quarantine` |
| `-convert` | `true` | converter mobi/azw3 |
| `-workers` | `NumCPU-2` | paralelismo |

## Estado

Funciona ponta a ponta: indexação incremental, detecção de duplicata, metadados, layout
para Kavita e Calibre, dry-run em tudo.

Falta: `-on-duplicate replace` (hoje só avisa), e a fase de metadados externos (gêneros e
ISBN via `fetch-ebook-metadata`), que fica de fora por custar 7–25 s por consulta.
