//go:build integration

package jobsrepo_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/assurrussa/goadmin/domain/outbox/repositories/jobsrepo"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	tests2 "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

var (
	name    = "job_name"
	payload = "job_payload"
)

type TestRepoSuite struct {
	suite.Suite

	db       outbox.StoragePgsqlClient
	dbHelper *tests.DBHelper
	cleanUp  func(context.Context)

	repo *jobsrepo.Repo
}

func NewTestRepoSuite(t *testing.T, opts ...tests.OptionDatabase) (context.Context, context.CancelFunc, *TestRepoSuite) {
	return tests2.NewSuite[*TestRepoSuite](t, func(t *testing.T, ctx context.Context) *TestRepoSuite {
		db, dbHelper, cleanUp := tests.PrepareDB(ctx, t, "TestJobsRepoSuite", opts...)
		outboxJobRepo := outbox.NewPgsqlJobsRepo(db)
		repo := jobsrepo.Must(jobsrepo.NewOptions(db, outboxJobRepo))

		return &TestRepoSuite{
			db:       db,
			dbHelper: dbHelper,
			cleanUp:  cleanUp,
			repo:     repo,
		}
	})
}

func TestIntegration_Init(t *testing.T) {
	assert.Panics(t, func() {
		jobsrepo.Must(jobsrepo.NewOptions(nil, nil))
	})
}

func TestIntegration_GetList(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)

	// Создаем тестовые данные
	data := createModels(t, ts, ctx, 10)

	ts.Run("get all rowModels with default pagination", func() {
		filters := createDefaultFilters(t)

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 10)

		// Проверяем что данные возвращаются
		ts.NotEmpty(rowModels[0].ID)
		ts.NotEmpty(rowModels[0].Name)
		ts.EqualValues(1, rowModels[0].SchemaVersion)
	})

	ts.Run("get rowModels with pagination", func() {
		filters := createDefaultFilters(t)
		filters.Page = 2
		filters.Limit = 3
		filters.SortOrder = "asc"

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)  // общее количество не изменилось
		ts.Len(rowModels, 3) // получили только 3 записи

		// Проверяем что это действительно вторая страница (записи 4, 5, 6)
		ts.Equal(data[3].ID, rowModels[0].ID)
		ts.Equal(data[4].ID, rowModels[1].ID)
		ts.Equal(data[5].ID, rowModels[2].ID)
	})

	ts.Run("get rowModels with sorting by username asc", func() {
		filters := createDefaultFilters(t)
		filters.SortBy = "name"
		filters.Page = 1
		filters.Limit = 5
		filters.SortOrder = "asc"

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 5)

		// Проверяем сортировку по username
		for i := 1; i < len(rowModels); i++ {
			ts.True(rowModels[i-1].Name <= rowModels[i].Name,
				"Name should be sorted in ascending order")
		}
	})

	ts.Run("get rowModels with invalid sort field falls back to default", func() {
		filters := createDefaultFilters(t)
		filters.Limit = 5
		filters.SortBy = "invalid_field" // недопустимое поле

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 5)
		// Должен использовать сортировку по умолчанию (created_at desc)
	})

	ts.Run("get rowModels with invalid sort order falls back to desc", func() {
		filters := createDefaultFilters(t)
		filters.Limit = 5
		filters.SortBy = "username"
		filters.SortOrder = "invalid" // недопустимый порядок

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 5)
		// Должен использовать desc как fallback
	})

	ts.Run("get rowModels with filters", func() {
		filters := createDefaultFilters(t)
		filters.Limit = 20
		filters.SortBy = "created_at"
		filters.SortOrder = "desc"
		filters.Fields = map[string]any{
			"name": "TestName___3",
		}

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(1, total)
		ts.Len(rowModels, 1)
		ts.Equal("TestName___3", rowModels[0].Name)
	})

	ts.Run("get rowModels with zero limit", func() {
		filters := createDefaultFilters(t)
		filters.Limit = 0 // нулевой лимит

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		// При нулевом лимите должны вернуться все записи - потому что по умолчанию 20 лимит
		ts.Len(rowModels, 10)
	})

	ts.Run("get rowModels with high offset", func() {
		filters := createDefaultFilters(t)
		filters.Page = 100 // очень большая страница

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 0) // должен вернуть пустой список
	})

	ts.Run("get rowModels with high offset", func() {
		filters := createDefaultFilters(t)
		filters.Page = -1 // отрицательный

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(10, total)
		ts.Len(rowModels, 10)
	})
}

func TestIntegration_CountExactAndAdministrativeDelete(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)

	jobs := createModels(t, ts, ctx, 2)

	count, err := ts.repo.CountExact(ctx)
	ts.Require().NoError(err)
	ts.EqualValues(2, count)

	affected, err := ts.repo.DeleteJob(ctx, jobs[0].ID)
	ts.Require().NoError(err)
	ts.EqualValues(1, affected)

	count, err = ts.repo.CountExact(ctx)
	ts.Require().NoError(err)
	ts.EqualValues(1, count)

	affected, err = ts.repo.DeleteJob(ctx, jobs[0].ID)
	ts.Require().NoError(err)
	ts.Zero(affected)
}

func createModels(t *testing.T, ts *TestRepoSuite, ctx context.Context, size int) []outbox.JobModel {
	t.Helper()

	tmCreate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	list := make([]outbox.JobModel, 0, 100)
	for i := 1; i <= size; i++ {
		strIndex := "__" + strconv.Itoa(i)
		rowModel := outbox.JobModel{
			ID:          outbox.NewJobID(),
			Queue:       "queue",
			Name:        "TestName_" + strIndex,
			Payload:     payload,
			Attempts:    i % 3,
			ReservedAt:  sql.NullTime{Valid: true, Time: tmCreate.Add(time.Duration(i) * time.Minute)}, // разные времена создания
			AvailableAt: tmCreate.Add(time.Duration(i) * time.Minute),                                  // разные времена создания
			CreatedAt:   tmCreate.Add(time.Duration(i) * time.Minute),                                  // разные времена создания
		}

		if i%3 == 0 {
			rowModel.ReservedAt = sql.NullTime{}
		}

		id, err := ts.repo.Create(ctx, rowModel)
		ts.Require().NoError(err)
		rowModel.ID = id

		list = append(list, rowModel)
	}

	return list
}

func createDefaultFilters(t *testing.T) datagrid.Filters {
	t.Helper()

	return datagrid.Filters{
		Page:      1,
		Limit:     10,
		SortBy:    "created_at",
		SortOrder: "desc",
		Fields:    make(map[string]any),
	}
}
