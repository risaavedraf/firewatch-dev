FROM golang:1.27.3-bookworm@sha256:972b4110ba0f47539a975c31e52c64765e1ff19f2d74469ae4a595c6676e80a5 AS build
WORKDIR /src/services/inference
ENV GOWORK=off CGO_ENABLED=0
COPY services/inference/go.mod services/inference/go.sum ./
RUN go mod download && go mod verify
COPY services/inference/ ./
RUN go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

FROM build AS test
# The contract test in internal/handler loads the published OpenAPI document
# through a repository-relative path, so the test stage has to carry it. Without
# this copy it fails with "no such file or directory" inside the container. The
# production stages do not need it, which is why it is added here and not in the
# build stage. The service has no dependency on contracts/gen: it reads the
# artifact JSON and exposes its own document.
COPY contracts/openapi/ /src/contracts/openapi/
ENTRYPOINT ["go", "test", "-count=1", "./..."]

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS runtime
COPY --from=build /server /server
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/server"]
