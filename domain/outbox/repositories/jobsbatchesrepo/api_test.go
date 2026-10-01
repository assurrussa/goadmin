//go:build integration

package jobsbatchesrepo_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/assurrussa/goadmin/domain/outbox/repositories/jobsbatchesrepo"
	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	tests2 "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	"github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/tests"
)

var (
	name       = "job_name"
	finishedAt = time.Now()
)

type TestRepoSuite struct {
	suite.Suite

	db       outbox.StoragePgsqlClient
	dbHelper *tests.DBHelper
	cleanUp  func(context.Context)

	repo *jobsbatchesrepo.Repo
}

func NewTestRepoSuite(t *testing.T, opts ...tests.OptionDatabase) (context.Context, context.CancelFunc, *TestRepoSuite) {
	return tests2.NewSuite[*TestRepoSuite](t, func(t *testing.T, ctx context.Context) *TestRepoSuite {
		db, dbHelper, cleanUp := tests.PrepareDB(ctx, t, "TestJobsFailedRepoSuite", opts...)
		repo := jobsbatchesrepo.Must(jobsbatchesrepo.NewOptions(db))

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
		outbox.NewPgsqlJobsRepo(nil)
	})
}

func TestIntegration_CreateFailedJob(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)

	jobExpected := createModel()
	jobID, err := ts.repo.Create(ctx, jobExpected)

	// Assert.
	ts.Require().NoError(err)

	// Checking if failed job was created.
	fJob, err := ts.repo.GetByID(ctx, jobID)
	ts.Require().NoError(err)
	ts.Require().NotNil(fJob)
	ts.NotEmpty(fJob.ID)
	ts.Equal(name, fJob.Name)
}

func TestIntegration_CreateFailedJob_Multiple(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)

	// Arrange.
	const fJobs = 3

	// Action.
	for i := 0; i < fJobs; i++ {
		jobExpected := createModel()
		_, err := ts.repo.Create(ctx, jobExpected)
		ts.Require().NoError(err)
	}

	// Assert.
	count, err := ts.repo.CountExact(ctx)
	ts.Require().NoError(err)
	ts.Equal(int64(fJobs), count)

	// light it's trigger in DB...
	time.Sleep(time.Millisecond * 2000)
	count, err = ts.repo.CountLight(ctx)
	ts.Require().NoError(err)
	ts.Equal(int64(fJobs), count)
}

func TestIntegration_DeleteJob(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)

	// Arrange.
	jobExpected := createModel()
	jobID, err := ts.repo.Create(ctx, jobExpected)
	ts.Require().NoError(err)
	ts.Require().NotEmpty(jobID)

	// Action.
	count, err := ts.repo.Delete(ctx, jobID)

	// Assert.
	ts.Require().NoError(err)
	ts.Equal(int64(1), count)

	// Checking if failed job was deleted.
	job, err := ts.repo.GetByID(ctx, jobID)
	ts.Require().ErrorIs(err, jobsbatchesrepo.ErrNoJobs)
	ts.Empty(job)
}

func TestIntegration_DeleteJob_NoJobs(t *testing.T) {
	ctx, _, ts := NewTestRepoSuite(t)
	defer ts.cleanUp(ctx)
	// Action.
	count, err := ts.repo.Delete(ctx, 9999)

	// Assert.
	ts.Require().NoError(err)
	ts.Equal(int64(0), count)
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
			ts.True(rowModels[i-1].ID <= rowModels[i].ID,
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

func createModel() outbox.JobBatchesModel {
	return outbox.JobBatchesModel{
		Name:         name,
		TotalJobs:    10,
		PendingJobs:  2,
		FailedJobs:   3,
		FailedJobIDs: "[1,3,7]",
		Options:      "",
		CancelledAt:  sql.NullTime{},
		CreatedAt:    finishedAt,
		FinishedAt:   sql.NullTime{Valid: true, Time: finishedAt},
	}
}

func createModels(t *testing.T, ts *TestRepoSuite, ctx context.Context, size int) []outbox.JobBatchesModel {
	t.Helper()

	tmCreate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	list := make([]outbox.JobBatchesModel, 0, 100)
	for i := 1; i <= size; i++ {
		strIndex := "__" + strconv.Itoa(i)
		rowModel := outbox.JobBatchesModel{
			Name:         "TestName_" + strIndex,
			TotalJobs:    int64(10 + i),
			PendingJobs:  int64(2 + i),
			FailedJobs:   int64(3 + i),
			FailedJobIDs: "[1,3,7]",
			Options:      "",
			CancelledAt:  sql.NullTime{Valid: true, Time: tmCreate.Add(time.Duration(i) * time.Minute)},
			CreatedAt:    tmCreate.Add(time.Duration(i) * time.Minute), // разные времена создания
			FinishedAt:   sql.NullTime{Valid: true, Time: tmCreate.Add(time.Duration(i) * time.Minute)},
		}

		if i%3 == 0 {
			rowModel.CancelledAt = sql.NullTime{}
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
		SortBy:    "id",
		SortOrder: "desc",
		Fields:    make(map[string]any),
	}
}
