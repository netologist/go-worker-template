# Technical Debt: Telemetry, Prometheus Metrics ve Dağıtık İzleme (Tracing) Eksikliği (TD-0002)

- **ID:** TD-0002
- **Tarih:** 2026-10-10
- **Durum:** Proposed / Backlog (Debt Recorded)
- **Öncelik:** High
- **İlgili Kapsam:** `internal/adapter/telemetry/`, `internal/config/`, `cmd/server/`, `cmd/worker/`, `docker-compose.yml`, `api/`
- **İlgili Standartlar:** OpenTelemetry (OTel) 1.30+, W3C TraceContext, B3 Propagation Spec, Prometheus OpenMetrics, OTLP v1

---

## 1. Problem ve Mevcut Durum

Mevcut şablonda (`go-worker-template`), temel yapılandırılmış loglama (`log/slog`) bulunmasına rağmen kurumsal bir mikrosistem için kritik olan şu gözlemlenebilirlik (observability) bileşenleri eksiktir:

1. **Metrik Ucu Yokluğu (`/metrics`):** Uygulama çalışma zamanına (Go runtime: goroutines, heap, GC pauses), HTTP istek trafiğine (throughput, latency, error rates) ve DB havuz istatistiklerine (aktif/boşta bağlantılar, bekleme süreleri) dair Prometheus metrikleri üretilmemektedir.
2. **Dağıtık İzleme (Distributed Tracing) Yokluğu:** İsteklerin servisler arası yolculuğunu uçtan uca takip edecek OTel Tracer altyapısı bulunmamaktadır; Jaeger veya Tempo gibi izleme arka uçlarına veri aktarılmamaktadır.
3. **Bağlam Taşıma (Context Propagation) Eksikliği:** Servisler arası çağrılarda hem modern W3C (`traceparent`) hem de kurumsal eski sistemlerin kullandığı B3 (`X-B3-TraceId`, `X-B3-SpanId`) başlıklarını okuyup downstream çağrılara iletecek bir composite propagator mekanizması tanımlanmamıştır.
4. **Log-Trace Korelasyonu Eksikliği:** `slog` çıktıları `trace_id` ve `span_id` taşımamaktadır; bu durum bir hata anında log satırından Jaeger izine veya izden ilişkili loglara zıplamayı (Trace-to-Logs) imkansız kılmaktadır.

---

## 2. Mimari Vizyon ve Modern Standartlar

Modern Go dünyasında doğrudan üçüncü parti istemciler (örneğin artık deprecated olan `jaeger-client-go` veya izole `prometheus/client_golang` enstrümantasyonu) yerine satıcıdan bağımsız **OpenTelemetry (OTel) Go SDK** kullanılır:
- **Metrik:** OTel `MeterProvider` + OTel Prometheus Exporter (Prometheus pull modeline uygun `/metrics`).
- **İzleme:** OTel `TracerProvider` + OTel OTLP gRPC Exporter (`otel-collector`, Jaeger veya Grafana Tempo için ortak protokol).
- **Bağlam Taşıma:** W3C TraceContext ve B3 Composite TextMapPropagator.
- **Log Korelasyonu:** Context'teki aktif spanden `trace_id` bilgisini çekip `slog.Record`'a enjekte eden middleware/handler.

---

## 3. Detaylandırılmış 8 Maddelik Uygulama Mimarisi

Aşağıdaki adımlar, hem senkron HTTP API modunda (`cmd/server`) hem de asenkron worker modunda (`cmd/worker`) tam uyumlu çalışacak şekilde tasarlanmıştır.

```
                  ┌──────────────────────────────────────────────┐
                  │          Inbound Request / Message           │
                  └──────────────────────┬───────────────────────┘
                                         │
                   (W3C: traceparent / B3: X-B3-TraceId)
                                         ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │ 1. Inbound Propagation (TextMapPropagator Extract)                          │
 └───────────────────────────────────────┬─────────────────────────────────────┘
                                         │ Go context.Context (WithSpanContext)
                                         ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │ 2. Telemetry Adapter Core (internal/adapter/telemetry/)                     │
 │  ┌───────────────────────────────────────────────────────────────────────┐  │
 │  │ TracerProvider (OTLP gRPC)    ──► Jaeger / Tempo (:4317)              │  │
 │  ├───────────────────────────────────────────────────────────────────────┤  │
 │  │ MeterProvider  (Prometheus)   ──► GET /metrics (Port 8080 veya 8081)  │  │
 │  ├───────────────────────────────────────────────────────────────────────┤  │
 │  │ slog Handler Interceptor      ──► JSON Log + {trace_id, span_id}      │  │
 │  └───────────────────────────────────────────────────────────────────────┘  │
 └───────────────────────────────────────┬─────────────────────────────────────┘
                                         │
                                         ▼
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │ 3. Outbound Propagation (Inject) ──► Downstream HTTP Client / Kafka / DB    │
 └─────────────────────────────────────────────────────────────────────────────┘
```

---

### Madde 1: Hexagonal Katmanlama & Dizin Yapısı (`internal/adapter/telemetry/`)

Telemetri, temiz mimaride bir **dış adaptör (adapter)** ve altyapı bileşenidir. Domain veya Usecase katmanları doğrudan OTel SDK'sına bağlanmamalı; yalnızca standart `context.Context` taşımalıdır.

```
internal/adapter/telemetry/
├── telemetry.go       # Provider ilklendirme, Shutdown lifecycle ve konfigürasyon
├── propagation.go     # W3C + B3 Composite Propagator kurulumu
├── tracing.go         # TracerProvider ve OTLP gRPC Exporter
├── metrics.go         # MeterProvider, Prometheus Exporter ve /metrics Handler
├── logger.go          # slog context interceptor (trace_id injection)
└── telemetry_test.go  # Context propagation ve shutdown testleri
```

---

### Madde 2: Bağımlılık Yönetimi (`go.mod`)

Deprecated kütüphaneler yerine OTel standart bileşenleri projeye eklenmelidir:

```bash
# 1. Çekirdek ve SDK
go get go.opentelemetry.io/otel@v1.30.0
go get go.opentelemetry.io/otel/sdk@v1.30.0
go get go.opentelemetry.io/otel/trace@v1.30.0
go get go.opentelemetry.io/otel/metric@v1.30.0

# 2. OTLP gRPC Exporter (Jaeger / Tempo / OpenTelemetry Collector için)
go get go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc@v1.30.0

# 3. Prometheus Exporter
go get go.opentelemetry.io/otel/exporters/prometheus@v0.52.0
go get github.com/prometheus/client_golang@v1.20.0

# 4. B3 & W3C Propagator
go get go.opentelemetry.io/contrib/propagators/b3@v1.30.0

# 5. HTTP Middleware Enstrümantasyonu
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.55.0
```

---

### Madde 3: Çift Yönlü Context Propagation (W3C + B3 Multi/Single)

Eski sistemler (Zipkin/Spring Cloud Sleuth: `X-B3-TraceId`, `X-B3-SpanId`, `X-B3-Sampled`) ile modern sistemler (W3C: `traceparent`, `tracestate`) arasında kayıpsız iletişim sağlamak için `CompositeTextMapPropagator` kurulur:

```go
package telemetry

import (
	"go.opentelemetry.io/contrib/propagators/b3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// SetupPropagator hem W3C hem B3 formatlarını anlayan global dağıtıcıyı kurar.
func SetupPropagator() {
	b3Propagator := b3.New(b3.WithInjectEncoding(b3.B3MultipleHeader | b3.B3SingleHeader))

	composite := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, // 1. W3C TraceContext (traceparent)
		propagation.Baggage{},      // 2. W3C Baggage (iş bağlamı)
		b3Propagator,               // 3. B3 (X-B3-*)
	)

	otel.SetTextMapPropagator(composite)
}
```

- **Inbound Davranışı:** İstek geldiğinde önce W3C aranır; yoksa B3 okunur; ikisi de yoksa yeni bir Root Trace oluşturulur.
- **Outbound Davranışı:** Downstream'e çağrı yapılırken hem `traceparent` hem de `X-B3-*` başlıkları otomatik basılır.

---

### Madde 4: TracerProvider ve OTLP gRPC Exporter (Jaeger/Tempo)

Jaeger 1.35+ ve Grafana Tempo, yerel olarak OTLP gRPC (port `4317`) dinler. `jaeger-client-go` yerine saf OTLP gRPC exporter kullanılmalıdır:

```go
package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func SetupTracer(ctx context.Context, cfg Config) (func(context.Context) error, error) {
	ctxTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exporter, err := otlptracegrpc.New(ctxTimeout,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithTLSCredentials(insecure.NewCredentials()), // Prod: mTLS veya TLS
		otlptracegrpc.WithDialOption(grpc.WithBlock()),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create telemetry resource: %w", err)
	}

	// Sampler stratejisi: Prodüksiyonda parent kararına uy, yeni köklerde oranlı örnekle
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))
	if cfg.Environment == "development" || cfg.Environment == "test" {
		sampler = sdktrace.AlwaysSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithMaxExportBatchSize(512),
		),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
```

---

### Madde 5: MeterProvider & Prometheus Exporter (`/metrics`)

Metriklerin hem OTel standart SDK sayaçları (counter, histogram) ile toplanması hem de Prometheus sunucuları tarafından `/metrics` üzerinden kazınabilmesi (scrape) sağlanır:

```go
package telemetry

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

func SetupMetrics() (http.Handler, func(context.Context) error, error) {
	// Prometheus okuyucusu ve exporter'ı
	exporter, err := otelprom.New()
	if err != nil {
		return nil, nil, fmt.Errorf("create prometheus exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(exporter),
	)
	otel.SetMeterProvider(mp)

	// Standart promhttp handler (OpenMetrics ve Prometheus formatını destekler)
	handler := promhttp.Handler()
	return handler, mp.Shutdown, nil
}
```

- **HTTP API Servisinde (`cmd/server`):** Mevcut router'a `router.Handle("/metrics", promHandler)` şeklinde eklenir.
- **Worker Servisinde (`cmd/worker`):** İş mantığından izole olan probe/admin HTTP sunucusuna (port `8081`) `/metrics` ucu olarak eklenir.

---

### Madde 6: Yapılandırılmış Log Korelasyonu (`slog` ile Trace-to-Log)

Loglar ile izler arasındaki bağı kurmak için `slog.Handler` sarmalanır. Her log satırına bağlamdaki span'in `trace_id` ve `span_id` değerleri otomatik olarak eklenir:

```go
package telemetry

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type SlogTraceHandler struct {
	slog.Handler
}

func NewSlogTraceHandler(inner slog.Handler) *SlogTraceHandler {
	return &SlogTraceHandler{Handler: inner}
}

func (h *SlogTraceHandler) Handle(ctx context.Context, r slog.Record) error {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		r.AddAttrs(
			slog.String("trace_id", span.SpanContext().TraceID().String()),
			slog.String("span_id", span.SpanContext().SpanID().String()),
			slog.Bool("trace_sampled", span.SpanContext().IsSampled()),
		)
	}
	return h.Handler.Handle(ctx, r)
}
```

Çıktı Örneği:
```json
{
  "time": "2026-10-10T22:00:00Z",
  "level": "INFO",
  "msg": "hotel created successfully",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "trace_sampled": true,
  "hotel_id": "019273c5-9b22-7934-9128-4efc354728f3"
}
```

---

### Madde 7: Taşıma Katmanı Enstrümantasyonu (HTTP & Kuyruklar)

#### A. HTTP Middleware Enstrümantasyonu:
`otelhttp` middleware'i gelen tüm HTTP isteklerinde otomatik span başlatır, gelen header'ları extract eder ve süre/durum kodlarını OTel metriklerine döker:

```go
// internal/adapter/httpapi/router.go
r.Use(func(next http.Handler) http.Handler {
    return otelhttp.NewHandler(next, "http-server",
        otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
    )
})
```

#### B. Asenkron Mesajlaşma (Kafka / RabbitMQ / SQS) Enstrümantasyonu:
Worker yapısında mesaj başlıklarından bağlamı çıkartıp yeni span açma örneği:

```go
func (c *KafkaConsumer) handleMessage(ctx context.Context, msg *kgo.Record) error {
    // 1. Header'ları MapCarrier üzerinden oku
    carrier := propagation.MapCarrier{}
    for _, h := range msg.Headers {
        carrier[h.Key] = string(h.Value)
    }

    // 2. Context'e bağla (W3C veya B3)
    ctx = otel.GetTextMapPropagator().Extract(ctx, carrier)

    // 3. Worker işlem span'ini başlat
    tr := otel.Tracer("kafka-consumer")
    ctx, span := tr.Start(ctx, "process-message",
        trace.WithAttributes(
            attribute.String("messaging.system", "kafka"),
            attribute.String("messaging.destination", msg.Topic),
            attribute.Int("messaging.kafka.partition", int(msg.Partition)),
            attribute.Int64("messaging.kafka.offset", msg.Offset),
        ),
    )
    defer span.End()

    return c.processor.Process(ctx, domainEvent)
}
```

---

### Madde 8: Konfigürasyon, Altyapı ve Kapatma Sıralaması

#### 1. Ortam Değişkenleri (`internal/config/config.go`):
```go
type TelemetryConfig struct {
    Enabled         bool    `env:"OTEL_ENABLED" envDefault:"true"`
    ServiceName     string  `env:"OTEL_SERVICE_NAME" envDefault:"service-template"`
    OTLPEndpoint    string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:"localhost:4317"`
    SampleRatio     float64 `env:"OTEL_SAMPLE_RATIO" envDefault:"1.0"`
    MetricsPath     string  `env:"METRICS_PATH" envDefault:"/metrics"`
}
```

#### 2. Kapatma Sıralaması (Graceful Shutdown Koreografisi):
`main.go` içinde kapatma anında tamponlanmış span'lerin kaybolmaması için:
```go
// 1. Sunucu / Consumer yeni istek alımını durdurur.
// 2. In-flight işlemler tamamlanır.
// 3. Telemetry Flush & Shutdown çağrılır:
if err := traceShutdown(shutdownCtx); err != nil {
    slog.Error("error shutting down tracer provider", "error", err)
}
if err := meterShutdown(shutdownCtx); err != nil {
    slog.Error("error shutting down meter provider", "error", err)
}
// 4. Veritabanı havuzu kapatılır.
```

#### 3. Yerel Test Ortamı (`docker-compose.yml`):
```yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:1.60
    container_name: local-jaeger
    ports:
      - "16686:16686" # Web UI
      - "4317:4317"   # OTLP gRPC alıcısı
      - "4318:4318"   # OTLP HTTP alıcısı
    environment:
      - COLLECTOR_OTLP_ENABLED=true

  prometheus:
    image: prom/prometheus:v2.54.1
    container_name: local-prometheus
    ports:
      - "9090:9090"
    volumes:
      - ./deploy/prometheus.yml:/etc/prometheus/prometheus.yml
```

---

## 4. Kabul Kriterleri (Definition of Done)

- [ ] `internal/adapter/telemetry/` paketi oluşturulmalı ve `go.mod` OTel bileşenleri eklenmeli.
- [ ] Hem W3C `traceparent` hem B3 `X-B3-*` başlıklarını çözen testler (`telemetry_test.go`) yazılmalı (%100 geçmeli).
- [ ] `/metrics` ucu Prometheus formatında Go runtime ve HTTP metriklerini eksiksiz sunmalı.
- [ ] `docker-compose up` ile Jaeger ayağa kaldırıldığında atılan HTTP istekleri Jaeger UI (`http://localhost:16686`) üzerinde span hiyerarşisiyle görünmeli.
- [ ] Uygulama JSON loglarında `trace_id` ve `span_id` alanlarının basıldığı doğrulanmalı.
- [ ] `SIGTERM` sinyalinde `TracerProvider.Shutdown` çağrılarak tamponlanmış span'lerin Jaeger'a başarıyla boşaltıldığı teyit edilmeli.
