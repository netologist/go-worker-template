package e2e_test

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Asynchronous Task Processing E2E", func() {
	It("processes an outbox task and updates status to COMPLETED", func() {
		if dbPool == nil {
			Skip("DATABASE_URL not configured; skipping database outbox integration test")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		taskID := uuid.New()
		taskType := "report.generate"
		payload := `{"report_type":"monthly_summary"}`

		// Insert a pending task into outbox
		_, err := dbPool.Exec(ctx, `
			INSERT INTO tasks (id, type, payload, status, result, created_at, updated_at)
			VALUES ($1, $2, $3, 'PENDING', '', NOW(), NOW())
		`, taskID, taskType, payload)
		Expect(err).NotTo(HaveOccurred())

		// Eventually verify the worker picked up the task and marked it COMPLETED
		Eventually(func(g Gomega) {
			var status string
			var result string
			queryErr := dbPool.QueryRow(ctx, `
				SELECT status, result FROM tasks WHERE id = $1
			`, taskID).Scan(&status, &result)

			g.Expect(queryErr).NotTo(HaveOccurred())
			g.Expect(status).To(Equal("COMPLETED"))
			g.Expect(result).To(ContainSubstring("processed report.generate task successfully"))
		}).WithTimeout(15 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		// Verify idempotency record in processed_messages
		var msgStatus string
		idempErr := dbPool.QueryRow(ctx, `
			SELECT status FROM processed_messages WHERE message_id = $1
		`, taskID.String()).Scan(&msgStatus)
		Expect(idempErr).NotTo(HaveOccurred())
		Expect(msgStatus).To(Equal("COMPLETED"))
	})
})
