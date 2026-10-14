FROM golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
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
