# Imagen de la aplicación. Dos etapas: se compila con el compilador de Go y se publica una imagen
# mínima que solo lleva el binario, los archivos estáticos, las fuentes de los PDF y los certificados raíz.
#
#   docker build -t cxc:ultima .

FROM golang:1.27-alpine AS construccion
WORKDIR /origen

# Las dependencias se descargan primero y en su propia capa: mientras go.mod no cambie, una
# recompilación no vuelve a bajarlas.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# La versión queda escrita en el binario y la devuelve /api/salud, para saber qué está corriendo.
ARG VERSION=desarrollo
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /cxc .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 aplicacion
WORKDIR /aplicacion

COPY --from=construccion /cxc /aplicacion/cxc
# El frontend se lee de disco (ver CLAUDE.md §11): va dentro de la imagen, no en un volumen.
COPY static /aplicacion/static
# Los PDF leen las fuentes y el logo de disco al generarlos (documentos.go).
COPY assets/pdf /aplicacion/assets/pdf

# Los adjuntos SÍ son datos: se montan como volumen, no viven en la imagen.
RUN mkdir -p /datos/archivos && chown -R aplicacion:aplicacion /datos
ENV RUTA_ARCHIVOS=/datos/archivos

USER aplicacion
EXPOSE 8080

# El contenedor se declara enfermo si la aplicación no puede hablar con la base: así el
# reverse proxy deja de mandarle peticiones y el reinicio automático se dispara.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget --quiet --spider http://127.0.0.1:8080/api/salud || exit 1

ENTRYPOINT ["/aplicacion/cxc"]
