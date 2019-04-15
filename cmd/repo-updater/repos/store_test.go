package repos_test

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/sourcegraph/sourcegraph/cmd/repo-updater/repos"
	"github.com/sourcegraph/sourcegraph/pkg/api"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc/bitbucketserver"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc/github"
	"github.com/sourcegraph/sourcegraph/pkg/extsvc/gitlab"
	log15 "gopkg.in/inconshreveable/log15.v2"
)

func TestFakeStore(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		test func(repos.Store) func(*testing.T)
	}{
		{"ListExternalServices", testStoreListExternalServices},
		{"UpsertExternalServices", testStoreUpsertExternalServices},
		{"ListRepos", testStoreListRepos},
		{"UpsertRepos", testStoreUpsertRepos},
	} {
		t.Run(tc.name, tc.test(repos.NewObservedStore(
			new(repos.FakeStore),
			log15.Root(),
		)))
	}
}

func testStoreListExternalServices(store repos.Store) func(*testing.T) {
	clock := repos.NewFakeClock(time.Now(), 0)
	now := clock.Now()

	github := repos.ExternalService{
		Kind:        "GITHUB",
		DisplayName: "Github - Test",
		Config:      `{"url": "https://github.com"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	gitlab := repos.ExternalService{
		Kind:        "GITLAB",
		DisplayName: "GitLab - Test",
		Config:      `{"url": "https://gitlab.com"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	bitbucketServer := repos.ExternalService{
		Kind:        "BITBUCKETSERVER",
		DisplayName: "Bitbucket Server - Test",
		Config:      `{"url": "https://bitbucketserver.mycorp.com"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	otherService := repos.ExternalService{
		Kind:        "OTHER",
		DisplayName: "Other code hosts",
		Config:      `{"url": "https://git-host.mycorp.com"}`,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	svcs := repos.ExternalServices{
		&github,
		&gitlab,
		&bitbucketServer,
		&otherService,
	}

	type testCase struct {
		name   string
		args   func(stored repos.ExternalServices) repos.StoreListExternalServicesArgs
		stored repos.ExternalServices
		assert repos.ExternalServicesAssertion
		err    error
	}

	var testCases []testCase
	testCases = append(testCases,
		testCase{
			name: "returned kind is uppercase",
			args: func(repos.ExternalServices) repos.StoreListExternalServicesArgs {
				return repos.StoreListExternalServicesArgs{
					Kinds: svcs.Kinds(),
				}
			},
			stored: svcs,
			assert: repos.Assert.ExternalServicesEqual(svcs...),
		},
		testCase{
			name: "case-insensitive kinds",
			args: func(repos.ExternalServices) (args repos.StoreListExternalServicesArgs) {
				for _, kind := range svcs.Kinds() {
					args.Kinds = append(args.Kinds, strings.ToLower(kind))
				}
				return args
			},
			stored: svcs,
			assert: repos.Assert.ExternalServicesEqual(svcs...),
		},
		testCase{
			name:   "excludes soft deleted external services by default",
			stored: svcs.With(repos.Opt.ExternalServiceDeletedAt(now)),
			assert: repos.Assert.ExternalServicesEqual(),
		},
		testCase{
			name:   "results are in ascending order by id",
			stored: mkExternalServices(512, svcs...),
			assert: repos.Assert.ExternalServicesOrderedBy(
				func(a, b *repos.ExternalService) bool {
					return a.ID < b.ID
				},
			),
		},
	)

	testCases = append(testCases, testCase{
		name:   "returns svcs by their ids",
		stored: svcs,
		args: func(stored repos.ExternalServices) repos.StoreListExternalServicesArgs {
			return repos.StoreListExternalServicesArgs{
				IDs: []int64{stored[0].ID, stored[1].ID},
			}
		},
		assert: repos.Assert.ExternalServicesEqual(svcs[:2].Clone()...),
	})

	return func(t *testing.T) {
		t.Helper()

		for _, tc := range testCases {
			tc := tc
			ctx := context.Background()

			t.Run(tc.name, transact(ctx, store, func(t testing.TB, tx repos.Store) {
				stored := tc.stored.Clone()
				if err := tx.UpsertExternalServices(ctx, stored...); err != nil {
					t.Fatalf("failed to setup store: %v", err)
				}

				var args repos.StoreListExternalServicesArgs
				if tc.args != nil {
					args = tc.args(stored)
				}

				es, err := tx.ListExternalServices(ctx, args)
				if have, want := fmt.Sprint(err), fmt.Sprint(tc.err); have != want {
					t.Errorf("error:\nhave: %v\nwant: %v", have, want)
				}

				if tc.assert != nil {
					tc.assert(t, es)
				}
			}))
		}
	}
}

func testStoreUpsertExternalServices(store repos.Store) func(*testing.T) {
	clock := repos.NewFakeClock(time.Now(), 0)
	now := clock.Now()

	return func(t *testing.T) {
		t.Helper()

		github := repos.ExternalService{
			Kind:        "GITHUB",
			DisplayName: "Github - Test",
			Config:      `{"url": "https://github.com"}`,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		gitlab := repos.ExternalService{
			Kind:        "GITLAB",
			DisplayName: "GitLab - Test",
			Config:      `{"url": "https://gitlab.com"}`,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		bitbucketServer := repos.ExternalService{
			Kind:        "BITBUCKETSERVER",
			DisplayName: "Bitbucket Server - Test",
			Config:      `{"url": "https://bitbucketserver.mycorp.com"}`,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		otherService := repos.ExternalService{
			Kind:        "OTHER",
			DisplayName: "Other code hosts",
			Config:      `{"url": "https://git-host.mycorp.com"}`,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		svcs := repos.ExternalServices{
			&github,
			&gitlab,
			&bitbucketServer,
			&otherService,
		}

		ctx := context.Background()

		t.Run("no external services", func(t *testing.T) {
			if err := store.UpsertExternalServices(ctx); err != nil {
				t.Fatalf("UpsertExternalServices error: %s", err)
			}
		})

		t.Run("many external services", transact(ctx, store, func(t testing.TB, tx repos.Store) {
			// Test more than one page load
			want := mkExternalServices(512, svcs...)

			if err := tx.UpsertExternalServices(ctx, want...); err != nil {
				t.Fatalf("UpsertExternalServices error: %s", err)
			}

			for _, e := range want {
				if e.Kind != strings.ToUpper(e.Kind) {
					t.Errorf("external service kind didn't get upper-cased: %q", e.Kind)
					break
				}
			}

			sort.Sort(want)

			have, err := tx.ListExternalServices(ctx, repos.StoreListExternalServicesArgs{
				Kinds: svcs.Kinds(),
			})

			if err != nil {
				t.Fatalf("ListExternalServices error: %s", err)
			}

			if diff := pretty.Compare(have, want); diff != "" {
				t.Fatalf("ListExternalServices:\n%s", diff)
			}

			now := clock.Now()
			suffix := "-updated"
			for _, r := range want {
				r.DisplayName += suffix
				r.Kind += suffix
				r.Config += suffix
				r.UpdatedAt = now
				r.CreatedAt = now
			}

			if err = tx.UpsertExternalServices(ctx, want...); err != nil {
				t.Errorf("UpsertExternalServices error: %s", err)
			} else if have, err = tx.ListExternalServices(ctx, repos.StoreListExternalServicesArgs{}); err != nil {
				t.Errorf("ListExternalServices error: %s", err)
			} else if diff := pretty.Compare(have, want); diff != "" {
				t.Errorf("ListExternalServices:\n%s", diff)
			}

			want.Apply(repos.Opt.ExternalServiceDeletedAt(now))
			args := repos.StoreListExternalServicesArgs{}

			if err = tx.UpsertExternalServices(ctx, want.Clone()...); err != nil {
				t.Errorf("UpsertExternalServices error: %s", err)
			} else if have, err = tx.ListExternalServices(ctx, args); err != nil {
				t.Errorf("ListExternalServices error: %s", err)
			} else if diff := pretty.Compare(have, repos.ExternalServices{}); diff != "" {
				t.Errorf("ListExternalServices:\n%s", diff)
			}
		}))
	}
}

func testStoreUpsertRepos(store repos.Store) func(*testing.T) {
	clock := repos.NewFakeClock(time.Now(), 0)
	now := clock.Now()

	return func(t *testing.T) {
		t.Helper()

		kinds := []string{
			"github",
			"gitlab",
			"bitbucketserver",
			"other",
		}

		github := repos.Repo{
			Name:        "github.com/foo/bar",
			Description: "The description",
			Language:    "barlang",
			Enabled:     true,
			CreatedAt:   now,
			ExternalRepo: api.ExternalRepoSpec{
				ID:          "AAAAA==",
				ServiceType: "github",
				ServiceID:   "http://github.com",
			},
			Sources: map[string]*repos.SourceInfo{
				"extsvc:1": {
					ID:       "extsvc:1",
					CloneURL: "git@github.com:foo/bar.git",
				},
			},
			Metadata: new(github.Repository),
		}

		gitlab := repos.Repo{
			Name:        "gitlab.com/foo/bar",
			Description: "The description",
			Language:    "barlang",
			Enabled:     true,
			CreatedAt:   now,
			ExternalRepo: api.ExternalRepoSpec{
				ID:          "1234",
				ServiceType: "gitlab",
				ServiceID:   "http://gitlab.com",
			},
			Sources: map[string]*repos.SourceInfo{
				"extsvc:2": {
					ID:       "extsvc:2",
					CloneURL: "git@gitlab.com:foo/bar.git",
				},
			},
			Metadata: new(gitlab.Project),
		}

		bitbucketServer := repos.Repo{
			Name:        "bitbucketserver.mycorp.com/foo/bar",
			Description: "The description",
			Language:    "barlang",
			Enabled:     true,
			CreatedAt:   now,
			ExternalRepo: api.ExternalRepoSpec{
				ID:          "1234",
				ServiceType: "bitbucketServer",
				ServiceID:   "http://bitbucketserver.mycorp.com",
			},
			Sources: map[string]*repos.SourceInfo{
				"extsvc:3": {
					ID:       "extsvc:3",
					CloneURL: "git@bitbucketserver.mycorp.com:foo/bar.git",
				},
			},
			Metadata: new(bitbucketserver.Repo),
		}

		otherRepo := repos.Repo{
			Name: "git-host.com/org/foo",
			ExternalRepo: api.ExternalRepoSpec{
				ID:          "git-host.com/org/foo",
				ServiceID:   "https://git-host.com/",
				ServiceType: "other",
			},
			Sources: map[string]*repos.SourceInfo{
				"extsvc:4": {
					ID:       "extsvc:3",
					CloneURL: "https://git-host.com/org/foo",
				},
			},
		}

		repositories := repos.Repos{
			&github,
			&gitlab,
			&bitbucketServer,
			&otherRepo,
		}

		ctx := context.Background()

		t.Run("no repos", func(t *testing.T) {
			if err := store.UpsertRepos(ctx); err != nil {
				t.Fatalf("UpsertRepos error: %s", err)
			}
		})

		t.Run("many repos", transact(ctx, store, func(t testing.TB, tx repos.Store) {
			// Test more than one page load
			want := mkRepos(512, repositories...)

			if err := tx.UpsertRepos(ctx, want...); err != nil {
				t.Fatalf("UpsertRepos error: %s", err)
			}

			sort.Sort(want)

			have, err := tx.ListRepos(ctx, repos.StoreListReposArgs{
				Kinds: kinds,
			})

			if err != nil {
				t.Fatalf("ListRepos error: %s", err)
			}

			if diff := pretty.Compare(have, want); diff != "" {
				t.Fatalf("ListRepos:\n%s", diff)
			}

			suffix := "-updated"
			now := clock.Now()
			for _, r := range want {
				r.Name += suffix
				r.Description += suffix
				r.Language += suffix
				r.UpdatedAt = now
				r.CreatedAt = now
				r.Archived = !r.Archived
				r.Fork = !r.Fork
			}

			if err = tx.UpsertRepos(ctx, want.Clone()...); err != nil {
				t.Errorf("UpsertRepos error: %s", err)
			} else if have, err = tx.ListRepos(ctx, repos.StoreListReposArgs{}); err != nil {
				t.Errorf("ListRepos error: %s", err)
			} else if diff := pretty.Compare(have, want); diff != "" {
				t.Errorf("ListRepos:\n%s", diff)
			}

			want.Apply(repos.Opt.RepoDeletedAt(now))
			args := repos.StoreListReposArgs{Deleted: true}

			if err = tx.UpsertRepos(ctx, want.Clone()...); err != nil {
				t.Errorf("UpsertRepos error: %s", err)
			} else if have, err = tx.ListRepos(ctx, args); err != nil {
				t.Errorf("ListRepos error: %s", err)
			} else if diff := pretty.Compare(have, want); diff != "" {
				t.Errorf("ListRepos:\n%s", diff)
			}

		}))
	}
}

func testStoreListRepos(store repos.Store) func(*testing.T) {
	clock := repos.NewFakeClock(time.Now(), 0)
	now := clock.Now()

	unmanaged := repos.Repo{
		Name:     "unmanaged",
		Sources:  map[string]*repos.SourceInfo{},
		Metadata: new(github.Repository),
		ExternalRepo: api.ExternalRepoSpec{
			ServiceType: "non_existent_kind",
			ServiceID:   "https://example.com/",
			ID:          "unmanaged",
		},
	}

	github := repos.Repo{
		Name: "github.com/bar/foo",
		Sources: map[string]*repos.SourceInfo{
			"extsvc:123": {
				ID:       "extsvc:123",
				CloneURL: "git@github.com:bar/foo.git",
			},
		},
		Metadata: new(github.Repository),
		ExternalRepo: api.ExternalRepoSpec{
			ServiceType: "github",
			ServiceID:   "https://github.com/",
			ID:          "foo",
		},
	}

	gitlab := repos.Repo{
		Name: "gitlab.com/bar/foo",
		Sources: map[string]*repos.SourceInfo{
			"extsvc:123": {
				ID:       "extsvc:123",
				CloneURL: "git@gitlab.com:bar/foo.git",
			},
		},
		Metadata: new(gitlab.Project),
		ExternalRepo: api.ExternalRepoSpec{
			ServiceType: "gitlab",
			ServiceID:   "https://gitlab.com/",
			ID:          "123",
		},
	}

	bitbucketServer := repos.Repo{
		Name: "bitbucketserver.mycorp.com/foo/bar",
		Sources: map[string]*repos.SourceInfo{
			"extsvc:123": {
				ID:       "extsvc:123",
				CloneURL: "git@bitbucketserver.mycorp.com:foo/bar.git",
			},
		},
		ExternalRepo: api.ExternalRepoSpec{
			ID:          "1234",
			ServiceType: "bitbucketServer",
			ServiceID:   "http://bitbucketserver.mycorp.com",
		},
		Metadata: new(bitbucketserver.Repo),
	}

	otherRepo := repos.Repo{
		Name: "git-host.com/org/foo",
		ExternalRepo: api.ExternalRepoSpec{
			ID:          "git-host.com/org/foo",
			ServiceID:   "https://git-host.com/",
			ServiceType: "other",
		},
		Sources: map[string]*repos.SourceInfo{
			"extsvc:4": {
				ID:       "extsvc:4",
				CloneURL: "https://git-host.com/org/foo",
			},
		},
	}

	repositories := repos.Repos{
		&github,
		&gitlab,
		&bitbucketServer,
		&otherRepo,
	}

	kinds := []string{
		"github",
		"gitlab",
		"bitbucketserver",
		"other",
	}

	type testCase struct {
		name   string
		args   func(stored repos.Repos) repos.StoreListReposArgs
		stored repos.Repos
		repos  repos.ReposAssertion
		err    error
	}

	var testCases []testCase
	{
		stored := repositories.With(func(r *repos.Repo) {
			r.ExternalRepo.ServiceType =
				strings.ToUpper(r.ExternalRepo.ServiceType)
		})

		testCases = append(testCases, testCase{
			name: "case-insensitive kinds",
			args: func(_ repos.Repos) (args repos.StoreListReposArgs) {
				for _, kind := range kinds {
					args.Kinds = append(args.Kinds, strings.ToUpper(kind))
				}
				return args
			},
			stored: stored,
			repos:  repos.Assert.ReposEqual(stored...),
		})
	}

	testCases = append(testCases, testCase{
		name: "ignores unmanaged",
		args: func(_ repos.Repos) repos.StoreListReposArgs {
			return repos.StoreListReposArgs{Kinds: kinds}
		},
		stored: repos.Repos{&github, &gitlab, &unmanaged}.Clone(),
		repos:  repos.Assert.ReposEqual(&github, &gitlab),
	})

	{
		stored := repositories.With(repos.Opt.RepoDeletedAt(now))
		testCases = append(testCases, testCase{
			name:   "excludes soft deleted repos by default",
			stored: stored,
			repos:  repos.Assert.ReposEqual(),
		})
	}

	{
		stored := repositories.With(repos.Opt.RepoDeletedAt(now))
		testCases = append(testCases, testCase{
			name: "includes soft deleted repos",
			args: func(repos.Repos) repos.StoreListReposArgs {
				return repos.StoreListReposArgs{Deleted: true}
			},
			stored: stored,
			repos:  repos.Assert.ReposEqual(stored...),
		})
	}

	testCases = append(testCases, testCase{
		name:   "returns repos in ascending order by id",
		stored: mkRepos(512, repositories...),
		repos: repos.Assert.ReposOrderedBy(func(a, b *repos.Repo) bool {
			return a.ID < b.ID
		}),
	})

	testCases = append(testCases, testCase{
		name:   "returns repos by their names",
		stored: repositories,
		args: func(_ repos.Repos) repos.StoreListReposArgs {
			return repos.StoreListReposArgs{
				Names: []string{github.Name, gitlab.Name},
			}
		},
		repos: repos.Assert.ReposEqual(&github, &gitlab),
	})

	testCases = append(testCases, testCase{
		name:   "returns repos by their ids",
		stored: repositories,
		args: func(stored repos.Repos) repos.StoreListReposArgs {
			return repos.StoreListReposArgs{
				IDs: []uint32{stored[0].ID, stored[1].ID},
			}
		},
		repos: repos.Assert.ReposEqual(repositories[:2].Clone()...),
	})

	testCases = append(testCases, testCase{
		name:   "limits repos to the given kinds",
		stored: repositories,
		args: func(repos.Repos) repos.StoreListReposArgs {
			return repos.StoreListReposArgs{
				Kinds: []string{"github", "gitlab"},
			}
		},
		repos: repos.Assert.ReposEqual(&github, &gitlab),
	})

	return func(t *testing.T) {
		t.Helper()

		for _, tc := range testCases {
			tc := tc
			ctx := context.Background()

			t.Run(tc.name, transact(ctx, store, func(t testing.TB, tx repos.Store) {
				stored := tc.stored.Clone()
				if err := tx.UpsertRepos(ctx, stored...); err != nil {
					t.Fatalf("failed to setup store: %v", err)
				}

				var args repos.StoreListReposArgs
				if tc.args != nil {
					args = tc.args(stored)
				}

				rs, err := tx.ListRepos(ctx, args)
				if have, want := fmt.Sprint(err), fmt.Sprint(tc.err); have != want {
					t.Errorf("error:\nhave: %v\nwant: %v", have, want)
				}

				if tc.repos != nil {
					tc.repos(t, rs)
				}
			}))
		}
	}
}

func testDBStoreTransact(store *repos.DBStore) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.Background()

		txstore, err := store.Transact(ctx)
		if err != nil {
			t.Fatal("expected DBStore to support transactions", err)
		}
		defer txstore.Done()

		_, err = txstore.(repos.Transactor).Transact(ctx)
		have := fmt.Sprintf("%s", err)
		want := "dbstore: already in a transaction"
		if have != want {
			t.Errorf("error:\nhave: %v\nwant: %v", have, want)
		}
	}
}

func mkRepos(n int, base ...*repos.Repo) repos.Repos {
	if len(base) == 0 {
		return nil
	}

	rs := make(repos.Repos, 0, n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		r := base[i%len(base)].Clone()
		r.Name += id
		r.ExternalRepo.ID += id
		rs = append(rs, r)
	}
	return rs
}

func mkExternalServices(n int, base ...*repos.ExternalService) repos.ExternalServices {
	if len(base) == 0 {
		return nil
	}
	es := make(repos.ExternalServices, 0, n)
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		r := base[i%len(base)].Clone()
		r.DisplayName += id
		es = append(es, r)
	}
	return es
}

func transact(ctx context.Context, s repos.Store, test func(testing.TB, repos.Store)) func(*testing.T) {
	return func(t *testing.T) {
		t.Helper()

		tr, ok := s.(repos.Transactor)

		if ok {
			txstore, err := tr.Transact(ctx)
			if err != nil {
				t.Fatalf("failed to start transaction: %v", err)
			}
			defer txstore.Done(&errRollback)
			s = &noopTxStore{TB: t, Store: txstore}
		}

		test(t, s)
	}
}

type noopTxStore struct {
	testing.TB
	repos.Store
	count int
}

func (tx *noopTxStore) Transact(context.Context) (repos.TxStore, error) {
	if tx.count != 0 {
		return nil, fmt.Errorf("noopTxStore: %d current transactions", tx.count)
	}
	tx.count++
	// noop
	return tx, nil
}

func (tx *noopTxStore) Done(errs ...*error) {
	tx.Helper()

	if tx.count != 1 {
		tx.Fatal("no current transactions")
	}
	if len(errs) > 0 && *errs[0] != nil {
		tx.Fatal(fmt.Sprintf("unexpected error in noopTxStore: %v", *errs[0]))
	}
	tx.count--
}
