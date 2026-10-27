FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
WORKDIR /src/services/gateway
ENV GOWORK=off CGO_ENABLED=0
COPY services/gateway/go.mod services/gateway/go.sum ./
RUN go mod download && go mod verify
COPY services/gateway/ ./
RUN go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM build AS test
ENTRYPOINT ["go", "test", "-count=1", "./..."]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime
COPY --from=build /server /server
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
