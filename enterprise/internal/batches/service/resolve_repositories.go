package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hashicorp/go-multierror"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/extsvc"
	"github.com/sourcegraph/sourcegraph/internal/gitserver"
	"github.com/sourcegraph/sourcegraph/internal/httpcli"
	streamapi "github.com/sourcegraph/sourcegraph/internal/search/streaming/api"
	streamhttp "github.com/sourcegraph/sourcegraph/internal/search/streaming/http"
	"github.com/sourcegraph/sourcegraph/internal/trace"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/internal/vcs/git"
	batcheslib "github.com/sourcegraph/sourcegraph/lib/batches"
)

type RepoRevision struct {
	Repo   *types.Repo
	Branch string
	Commit api.CommitID
}

func (r *RepoRevision) HasBranch() bool {
	return r.Branch != ""
}

type ResolveRepositoriesForBatchSpecOpts struct {
	AllowIgnored     bool
	AllowUnsupported bool
}

func (s *Service) ResolveRepositoriesForBatchSpec(ctx context.Context, batchSpec *batcheslib.BatchSpec, opts ResolveRepositoriesForBatchSpecOpts) (_ []*RepoRevision, err error) {
	traceTitle := fmt.Sprintf("len(On): %d", len(batchSpec.On))
	tr, ctx := trace.New(ctx, "service.ResolveRepositoriesForBatchSpec", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	seen := map[api.RepoID]*RepoRevision{}
	unsupported := UnsupportedRepoSet{}
	ignored := IgnoredRepoSet{}

	// TODO: this could be trivially parallelised in the future.
	for _, on := range batchSpec.On {
		repos, err := s.resolveRepositoriesOn(ctx, &on)
		if err != nil {
			return nil, errors.Wrapf(err, "resolving %q", on.String())
		}

		for _, repo := range repos {
			// Skip repos where no branch exists.
			if !repo.HasBranch() {
				continue
			}

			if other, ok := seen[repo.Repo.ID]; !ok {
				seen[repo.Repo.ID] = repo

				switch st := repo.Repo.ExternalRepo.ServiceType; st {
				case extsvc.TypeGitHub, extsvc.TypeGitLab, extsvc.TypeBitbucketServer:
				default:
					if !opts.AllowUnsupported {
						unsupported.Append(repo.Repo)
					}
				}
			} else {
				// If we've already seen this repository, we overwrite the
				// Commit/Branch fields with the latest value we have
				other.Commit = repo.Commit
				other.Branch = repo.Branch
			}
		}
	}

	final := make([]*RepoRevision, 0, len(seen))
	// TODO: Limit concurrency.
	var wg sync.WaitGroup
	var errs *multierror.Error
	for _, repo := range seen {
		repo := repo
		wg.Add(1)
		go func(repo *RepoRevision) {
			defer wg.Done()
			ignore, err := s.hasBatchIgnoreFile(ctx, repo)
			if err != nil {
				errs = multierror.Append(errs, err)
				return
			}
			if !opts.AllowIgnored && ignore {
				ignored.Append(repo.Repo)
			}

			if !unsupported.Includes(repo.Repo) && !ignored.Includes(repo.Repo) {
				final = append(final, repo)
			}
		}(repo)
	}
	wg.Wait()
	if err := errs.ErrorOrNil(); err != nil {
		return nil, err
	}

	if unsupported.HasUnsupported() {
		return final, unsupported
	}

	if ignored.HasIgnored() {
		return final, ignored
	}

	return final, nil
}

var ErrMalformedOnQueryOrRepository = errors.New("malformed 'on' field; missing either a repository name or a query")

func (s *Service) resolveRepositoriesOn(ctx context.Context, on *batcheslib.OnQueryOrRepository) (_ []*RepoRevision, err error) {
	traceTitle := fmt.Sprintf("On: %+v", on)
	tr, ctx := trace.New(ctx, "service.resolveRepositoriesOn", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	if on.RepositoriesMatchingQuery != "" {
		return s.resolveRepositorySearch(ctx, on.RepositoriesMatchingQuery)
	} else if on.Repository != "" && on.Branch != "" {
		repo, err := s.resolveRepositoryNameAndBranch(ctx, on.Repository, on.Branch)
		if err != nil {
			return nil, err
		}
		return []*RepoRevision{repo}, nil
	} else if on.Repository != "" {
		repo, err := s.resolveRepositoryName(ctx, on.Repository)
		if err != nil {
			return nil, err
		}
		return []*RepoRevision{repo}, nil
	}

	// This shouldn't happen on any batch spec that has passed validation, but,
	// alas, software.
	return nil, ErrMalformedOnQueryOrRepository
}

func (s *Service) resolveRepositoryName(ctx context.Context, name string) (_ *RepoRevision, err error) {
	traceTitle := fmt.Sprintf("Name: %q", name)
	tr, ctx := trace.New(ctx, "service.resolveRepositoryName", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	repo, err := s.store.Repos().GetByName(ctx, api.RepoName(name))
	if err != nil {
		return nil, err
	}

	return s.repoToRepoRevision(ctx, repo)
}

func (s *Service) repoToRepoRevision(ctx context.Context, repo *types.Repo) (_ *RepoRevision, err error) {
	traceTitle := fmt.Sprintf("Repo: %q", repo.Name)
	tr, ctx := trace.New(ctx, "service.resolveRepositoriesOn", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	repoRev := &RepoRevision{
		Repo: repo,
	}

	// TODO: Fill default branch.
	refBytes, _, exitCode, err := git.ExecSafe(ctx, repo.Name, []string{"symbolic-ref", "HEAD"})
	repoRev.Branch = string(bytes.TrimSpace(refBytes))
	if err == nil && exitCode == 0 {
		// Check that our repo is not empty
		repoRev.Commit, err = git.ResolveRevision(ctx, repo.Name, "HEAD", git.ResolveRevisionOptions{NoEnsureRevision: true})
	}
	// TODO: Handle repoCloneInProgressErr
	return repoRev, err
}

func (s *Service) resolveRepositoryNameAndBranch(ctx context.Context, name, branch string) (_ *RepoRevision, err error) {
	traceTitle := fmt.Sprintf("Name: %q Branch: %q", name, branch)
	tr, ctx := trace.New(ctx, "service.resolveRepositoryNameAndBranch", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	repo, err := s.resolveRepositoryName(ctx, name)
	if err != nil {
		return repo, err
	}

	commit, err := git.ResolveRevision(ctx, repo.Repo.Name, branch, git.ResolveRevisionOptions{
		NoEnsureRevision: true,
	})
	if err != nil && errors.HasType(err, &gitserver.RevisionNotFoundError{}) {
		return repo, fmt.Errorf("no branch matching %q found for repository %s", branch, name)
	}

	repo.Branch = branch
	repo.Commit = commit

	return repo, err
}

func (s *Service) resolveRepositorySearch(ctx context.Context, query string) (_ []*RepoRevision, err error) {
	traceTitle := fmt.Sprintf("Query: %q", query)
	tr, ctx := trace.New(ctx, "service.resolveRepositorySearch", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	query = setDefaultQueryCount(query)
	query = setDefaultQuerySelect(query)

	repoIDs := []api.RepoID{}
	s.runSearch(ctx, query, func(matches []streamhttp.EventMatch) {
		for _, match := range matches {
			switch m := match.(type) {
			case *streamhttp.EventRepoMatch:
				repoIDs = append(repoIDs, api.RepoID(m.RepositoryID))
			case *streamhttp.EventContentMatch:
				repoIDs = append(repoIDs, api.RepoID(m.RepositoryID))
			}
		}
	})

	accessibleRepos, err := s.store.Repos().List(ctx, database.ReposListOptions{IDs: repoIDs})
	if err != nil {
		return nil, err
	}
	revs := make([]*RepoRevision, 0, len(accessibleRepos))
	for _, repo := range accessibleRepos {
		rev, err := s.repoToRepoRevision(ctx, repo)
		if err != nil {
			{
				return nil, err
			}
		}
		revs = append(revs, rev)
	}

	return revs, nil
}

func (s *Service) runSearch(ctx context.Context, query string, onMatches func(matches []streamhttp.EventMatch)) (err error) {
	req, err := streamhttp.NewRequest(api.InternalClient.URL+"/.internal", query)
	if err != nil {
		return err
	}

	req.WithContext(ctx)
	// TODO: Document why it's okay to not pass along the ctx.User here.
	req.Header.Set("User-Agent", "Batch Changes repository resolver")

	resp, err := httpcli.InternalClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	dec := streamhttp.FrontendStreamDecoder{
		OnMatches: func(matches []streamhttp.EventMatch) {
			onMatches(matches)
		},
		OnError: func(ee *streamhttp.EventError) {
			err = errors.New(ee.Message)
		},
		OnProgress: func(p *streamapi.Progress) {
			// TODO: Evaluate skipped for values we care about.
		},
	}
	return dec.ReadAll(resp.Body)
}

func (s *Service) hasBatchIgnoreFile(ctx context.Context, r *RepoRevision) (_ bool, err error) {
	traceTitle := fmt.Sprintf("Repo: %q Revision: %q", r.Repo.Name, r.Branch)
	tr, ctx := trace.New(ctx, "service.hasBatchIgnoreFile", traceTitle)
	defer func() {
		tr.SetError(err)
		tr.Finish()
	}()

	path := ".batchignore"
	stat, err := git.Stat(ctx, r.Repo.Name, r.Commit, path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !stat.Mode().IsRegular() {
		return false, errors.Errorf("not a blob: %q", path)
	}
	return true, nil
}

var defaultQueryCountRegex = regexp.MustCompile(`\bcount:(\d+|all)\b`)

const hardCodedCount = " count:all"

func setDefaultQueryCount(query string) string {
	if defaultQueryCountRegex.MatchString(query) {
		return query
	}

	return query + hardCodedCount
}

var selectRegex = regexp.MustCompile(`\bselect:(.+)\b`)

const hardCodedSelectRepo = " select:repo"

func setDefaultQuerySelect(query string) string {
	if selectRegex.MatchString(query) {
		return query
	}

	return query + hardCodedSelectRepo
}

// TODO(mrnugget): Merge these two types (give them an "errorfmt" function,
// rename "Has*" methods to "NotEmpty" or something)

// UnsupportedRepoSet provides a set to manage repositories that are on
// unsupported code hosts. This type implements error to allow it to be
// returned directly as an error value if needed.
type UnsupportedRepoSet map[*types.Repo]struct{}

func (e UnsupportedRepoSet) Includes(r *types.Repo) bool {
	_, ok := e[r]
	return ok
}

func (e UnsupportedRepoSet) Error() string {
	repos := []string{}
	typeSet := map[string]struct{}{}
	for repo := range e {
		repos = append(repos, string(repo.Name))
		typeSet[repo.ExternalRepo.ServiceType] = struct{}{}
	}

	types := []string{}
	for t := range typeSet {
		types = append(types, t)
	}

	return fmt.Sprintf(
		"found repositories on unsupported code hosts: %s\nrepositories:\n\t%s",
		strings.Join(types, ", "),
		strings.Join(repos, "\n\t"),
	)
}

func (e UnsupportedRepoSet) Append(repo *types.Repo) {
	e[repo] = struct{}{}
}

func (e UnsupportedRepoSet) HasUnsupported() bool {
	return len(e) > 0
}

// IgnoredRepoSet provides a set to manage repositories that are on
// unsupported code hosts. This type implements error to allow it to be
// returned directly as an error value if needed.
type IgnoredRepoSet map[*types.Repo]struct{}

func (e IgnoredRepoSet) Includes(r *types.Repo) bool {
	_, ok := e[r]
	return ok
}

func (e IgnoredRepoSet) Error() string {
	repos := []string{}
	for repo := range e {
		repos = append(repos, string(repo.Name))
	}

	return fmt.Sprintf(
		"found repositories containing .batchignore files:\n\t%s",
		strings.Join(repos, "\n\t"),
	)
}

func (e IgnoredRepoSet) Append(repo *types.Repo) {
	e[repo] = struct{}{}
}

func (e IgnoredRepoSet) HasIgnored() bool {
	return len(e) > 0
}
