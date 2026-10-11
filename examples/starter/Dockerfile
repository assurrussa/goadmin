FROM node:24-alpine AS assets
WORKDIR /src/resources
COPY resources/package.json resources/package-lock.json ./
RUN npm ci
COPY resources/ ./
COPY views/ /src/views/
RUN npm run test:manifest && npm run test:unit && npm run type-check && npm run build-only

FROM golang:1.27.2-alpine AS build
RUN apk add --no-cache git
WORKDIR /src/goadmin
COPY . .
COPY --from=goauth . /src/goauth/
COPY --from=gonotify . /src/gonotify/
COPY --from=gouploads . /src/gouploads/
COPY --from=gowebsocket . /src/gowebsocket/
COPY --from=assets /src/public/dist /src/goadmin/public/dist
WORKDIR /src/goadmin/examples/starter
RUN GOWORK=off go build -mod=readonly -o /tmp/goadmin-starter .

FROM alpine:3.22
RUN addgroup -S app && adduser -S -G app app && mkdir -p /data/uploads && chown -R app:app /data
COPY --from=build /tmp/goadmin-starter /usr/local/bin/goadmin-starter
USER app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/goadmin-starter"]
