FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
WORKDIR /src/services/prevention
ENV GOWORK=off CGO_ENABLED=0
COPY services/prevention/go.mod services/prevention/go.sum ./
RUN go mod download && go mod verify
COPY services/prevention/ ./
RUN go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM build AS test
# The contract tests in internal/handler load the published OpenAPI document
# through a repository-relative path, so the test stage has to carry it. Without
# this copy they fail with "no such file or directory" inside the container, and
# the integration job - the only place the database-side rules run - cannot pass.
# The production stages do not need it, which is why it is added here and not in
# the build stage.
COPY contracts/openapi/ /src/contracts/openapi/
ENTRYPOINT ["go", "test", "-count=1", "./..."]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime
COPY --from=build /server /server
USER 65532:65532
# The service's own port, fixed by the frozen contract. It is not 8080, which
# belongs to the gateway on the local stack.
EXPOSE 8084
ENTRYPOINT ["/server"]
