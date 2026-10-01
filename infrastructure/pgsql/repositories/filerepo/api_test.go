//go:build integration

package filerepo_test

import (
	"context"
	"database/sql"
	"strconv"
	"testing"
	"time"

	uploadhost "github.com/assurrussa/gouploads/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/assurrussa/goadmin/infrastructure/core/datagrid"
	tests "github.com/assurrussa/goadmin/infrastructure/fiber/testsupport/utilst"
	outbox "github.com/assurrussa/goadmin/infrastructure/outbox"
	"github.com/assurrussa/goadmin/infrastructure/pgsql/repositories/filerepo"
	"github.com/assurrussa/goadmin/internal/pointer"
	pgsqltests "github.com/assurrussa/goadmin/tests"
)

type TestRepoSuite struct {
	suite.Suite

	db       outbox.StoragePgsqlClient
	dbHelper *pgsqltests.DBHelper
	trx      *outbox.PgsqlTrxManager
	cleanUp  func(context.Context)

	repo *filerepo.Repo
}

func NewTestRepoSuite(t *testing.T, opts ...pgsqltests.OptionDatabase) (context.Context, context.CancelFunc, *TestRepoSuite) {
	return tests.NewSuite[*TestRepoSuite](t, func(t *testing.T, ctx context.Context) *TestRepoSuite {
		db, dbHelper, cleanUp := pgsqltests.PrepareDB(ctx, t, "TestFilesRepoSuite", opts...)
		trx := outbox.PgsqlTrxNew(db.DB())
		fileRepoBase := &uploadhost.FileRepo{}
		repo := filerepo.Must(filerepo.NewOptions(db, trx, fileRepoBase))

		return &TestRepoSuite{
			db:       db,
			dbHelper: dbHelper,
			trx:      trx,
			cleanUp:  cleanUp,
			repo:     repo,
		}
	})
}

func TestIntegration_Init(t *testing.T) {
	assert.Panics(t, func() {
		filerepo.Must(filerepo.NewOptions(nil, nil, nil))
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
		ts.NotEmpty(rowModels[0].Slug)
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
				"Username should be sorted in ascending order")
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
		// Создаем админа с уникальным именем для фильтрации
		uniqueAdmin := uploadhost.File{
			Name:        "UniqueAdminName",
			Description: pointer.To("UniqueLastName"),
			Slug:        "unique@test.test",
			Locale:      pointer.To("EN_en"),
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
			PublishedAt: sql.NullTime{Valid: true, Time: time.Now()},
		}

		insertModel(t, ts, ctx, uniqueAdmin)

		filters := createDefaultFilters(t)
		filters.Limit = 20
		filters.SortBy = "created_at"
		filters.SortOrder = "desc"
		filters.Fields = map[string]any{
			"slug": "TestSlug___3",
		}

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(1, total)
		ts.Len(rowModels, 1)
		ts.Equal("TestSlug___3", rowModels[0].Slug)
	})

	ts.Run("get rowModels with zero limit", func() {
		filters := createDefaultFilters(t)
		filters.Limit = 0 // нулевой лимит

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(11, total)
		// При нулевом лимите должны вернуться все записи - потому что по умолчанию 20 лимит
		ts.Len(rowModels, 11)
	})

	ts.Run("get rowModels with high offset", func() {
		filters := createDefaultFilters(t)
		filters.Page = 100 // очень большая страница

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(11, total)
		ts.Len(rowModels, 0) // должен вернуть пустой список
	})

	ts.Run("get rowModels with high offset", func() {
		filters := createDefaultFilters(t)
		filters.Page = -1 // отрицательный

		rowModels, total, err := ts.repo.GetList(ctx, filters)
		ts.Require().NoError(err)
		ts.Equal(11, total)
		ts.Len(rowModels, 10)
	})
}

func createModels(t *testing.T, ts *TestRepoSuite, ctx context.Context, size int) []uploadhost.File {
	t.Helper()

	tmCreate := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	list := make([]uploadhost.File, 0, 100)
	for i := 1; i <= size; i++ {
		strIndex := "__" + strconv.Itoa(i)
		rowModel := uploadhost.File{
			Name:        "TestName_" + strIndex,
			Description: pointer.To("TestDesc_" + strIndex),
			Slug:        "TestSlug_" + strIndex,
			URL:         "TestUrl_" + strIndex,
			Locale:      pointer.To("EN_en"),
			CreatedAt:   tmCreate.Add(time.Duration(i) * time.Minute), // разные времена создания
			UpdatedAt:   tmCreate.Add(time.Duration(i) * time.Minute),
			PublishedAt: sql.NullTime{Valid: true, Time: tmCreate.Add(time.Duration(i) * time.Minute)},
		}

		if i%3 == 0 {
			rowModel.PublishedAt = sql.NullTime{}
		}

		rowModel.ID = insertModel(t, ts, ctx, rowModel)

		list = append(list, rowModel)
	}

	return list
}

func insertModel(t *testing.T, ts *TestRepoSuite, ctx context.Context, rowModel uploadhost.File) int64 {
	t.Helper()

	builder := outbox.BuilderDollar().
		Insert("files").
		Suffix("RETURNING id").
		SetMap(map[string]any{
			"user_id":           rowModel.UserID,
			"manager_id":        rowModel.ManagerID,
			"object_type":       rowModel.ObjectType,
			"object_id":         rowModel.ObjectID,
			"original_filename": rowModel.OriginalFileName,
			"filename":          rowModel.FileName,
			"folder_path":       rowModel.FolderPath,
			"provider":          rowModel.Provider,
			"size":              rowModel.Size,
			"mime_type":         rowModel.MimeType,
			"url":               rowModel.URL,
			"slug":              rowModel.Slug,
			"name":              rowModel.Name,
			"description":       rowModel.Description,
			"data":              rowModel.Data,
			"is_primary":        rowModel.IsPrimary,
			"created_at":        rowModel.CreatedAt,
			"updated_at":        rowModel.UpdatedAt,
			"published_at":      rowModel.PublishedAt,
		})

	var id int64
	require.NoError(t, ts.db.DB().Getx(ctx, "insert_test_file", &id, builder))

	return id
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
