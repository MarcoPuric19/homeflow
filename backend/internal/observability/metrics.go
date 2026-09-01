package observability

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry             *prometheus.Registry
	httpRequests         *prometheus.CounterVec
	httpDuration         *prometheus.HistogramVec
	dbDuration           *prometheus.HistogramVec
	loginFailures        prometheus.Counter
	shoppingItemsCreated prometheus.Counter
	expensesCreated      prometheus.Counter
	receiptDuration      prometheus.Histogram
}

func New() *Metrics {
	m := &Metrics{
		registry:             prometheus.NewRegistry(),
		httpRequests:         prometheus.NewCounterVec(prometheus.CounterOpts{Name: "homeflow_http_requests_total", Help: "HTTP requests grouped by stable route, method and status."}, []string{"route", "method", "status"}),
		httpDuration:         prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "homeflow_http_request_duration_seconds", Help: "HTTP request duration grouped by stable route and method.", Buckets: prometheus.DefBuckets}, []string{"route", "method"}),
		dbDuration:           prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "homeflow_db_query_duration_seconds", Help: "PostgreSQL query duration grouped by operation and outcome.", Buckets: prometheus.DefBuckets}, []string{"operation", "outcome"}),
		loginFailures:        prometheus.NewCounter(prometheus.CounterOpts{Name: "homeflow_login_failures_total", Help: "Rejected login attempts."}),
		shoppingItemsCreated: prometheus.NewCounter(prometheus.CounterOpts{Name: "homeflow_shopping_items_created_total", Help: "Shopping items created."}),
		expensesCreated:      prometheus.NewCounter(prometheus.CounterOpts{Name: "homeflow_expenses_created_total", Help: "Expenses created."}),
		receiptDuration:      prometheus.NewHistogram(prometheus.HistogramOpts{Name: "homeflow_receipt_processing_duration_seconds", Help: "Synchronous receipt validation and storage duration.", Buckets: prometheus.DefBuckets}),
	}
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), m.httpRequests, m.httpDuration, m.dbDuration, m.loginFailures, m.shoppingItemsCreated, m.expensesCreated, m.receiptDuration)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
func (m *Metrics) ObserveHTTP(route, method, status string, elapsed time.Duration) {
	if route == "" {
		route = "unmatched"
	}
	m.httpRequests.WithLabelValues(route, method, status).Inc()
	m.httpDuration.WithLabelValues(route, method).Observe(elapsed.Seconds())
}
func (m *Metrics) LoginFailed()                         { m.loginFailures.Inc() }
func (m *Metrics) ShoppingItemCreated()                 { m.shoppingItemsCreated.Inc() }
func (m *Metrics) ExpenseCreated()                      { m.expensesCreated.Inc() }
func (m *Metrics) ObserveReceipt(elapsed time.Duration) { m.receiptDuration.Observe(elapsed.Seconds()) }

func (m *Metrics) BindDatabase(pool *pgxpool.Pool) {
	m.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "homeflow_db_pool_acquired_connections", Help: "Currently acquired PostgreSQL connections."}, func() float64 { return float64(pool.Stat().AcquiredConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "homeflow_db_pool_idle_connections", Help: "Currently idle PostgreSQL connections."}, func() float64 { return float64(pool.Stat().IdleConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "homeflow_expenses_due", Help: "Open expenses whose due date is today or earlier."}, func() float64 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var count float64
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM expenses WHERE status IN ('planned','open') AND due_date <= CURRENT_DATE`).Scan(&count); err != nil {
				return 0
			}
			return count
		}),
	)
}

type queryStartedKey struct{}
type queryStarted struct {
	started   time.Time
	operation string
}

func (m *Metrics) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	operation := "OTHER"
	if fields := strings.Fields(data.SQL); len(fields) > 0 {
		operation = strings.ToUpper(fields[0])
	}
	return context.WithValue(ctx, queryStartedKey{}, queryStarted{time.Now(), operation})
}

func (m *Metrics) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	started, ok := ctx.Value(queryStartedKey{}).(queryStarted)
	if !ok {
		return
	}
	outcome := "success"
	if data.Err != nil {
		outcome = "error"
	}
	m.dbDuration.WithLabelValues(started.operation, outcome).Observe(time.Since(started.started).Seconds())
}
