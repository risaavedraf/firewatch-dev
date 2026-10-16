FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
WORKDIR /src/services/risk
ENV GOWORK=off CGO_ENABLED=0
COPY contracts/gen/ /src/contracts/gen/
COPY services/risk/go.mod services/risk/go.sum ./
RUN go mod download && go mod verify
COPY services/risk/ ./
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
EXPOSE 8080
ENTRYPOINT ["/server"]
