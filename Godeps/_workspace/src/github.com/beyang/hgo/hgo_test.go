package hgo_test

import (
	"testing"

	"github.com/beyang/hgo"
	hg_changelog "github.com/beyang/hgo/changelog"
	hg_revlog "github.com/beyang/hgo/revlog"
	hg_store "github.com/beyang/hgo/store"
)

var (
	touchTime = appleTime("2006-01-02T15:04:05Z")
	commands  = []string{
		"hg init",
		"touch --date=2006-01-02T15:04:05Z f || touch -t " + touchTime + " f",
		"hg add f",
		"hg commit -m foo --date '2006-12-06 13:18:29 UTC' --user 'a <a@a.com>'",
		// Some versions of Mercurial don't create .hg/cache until another command
		// is ran that uses branches. Ran into this on Mercurial 2.0.2.
		"hg branches >/dev/null",
	}
	revision = "e8e11ff1be92a7be71b9b5cdb4cc674b7dc9facf"
)

func TestOpenRepository(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	_, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}
}

func TestTags(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	globalTags, allTags := repo.Tags()
	if globalTags == nil {
		t.Fatal("Unable to get global tags")
	}
	if allTags == nil {
		t.Fatal("Unable to get all tags")
	}
}

func TestBranchHeads(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	_, err = repo.BranchHeads()
	if err != nil {
		t.Errorf("Unable to get branch heads: %s", err)
	}
}

func TestOpenChangeLog(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	_, err = s.OpenChangeLog()
	if err != nil {
		t.Fatalf("Unable to open change log: %s", err)
	}
}

func TestNewStore(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}
}

func TestStoreOpenRevlog(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	_, err = s.OpenRevlog("f")
	if err != nil {
		t.Fatalf("Unable to open revlog: %s", err)
	}
}

func TestAddTag(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	cl, err := s.OpenChangeLog()
	if err != nil {
		t.Fatalf("Unable to open change log: %s", err)
	}

	globalTags, allTags := repo.Tags()
	if globalTags == nil {
		t.Fatal("Unable to get global tags")
	}
	if allTags == nil {
		t.Fatal("Unable to get all tags")
	}

	globalTags.Sort()
	allTags.Sort()
	allTags.Add("tip", cl.Tip().Id().Node())
}

func TestLookup(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	cl, err := s.OpenChangeLog()
	if err != nil {
		t.Fatalf("Unable to open change log: %s", err)
	}

	_, err = hg_revlog.NodeIdRevSpec(revision).Lookup(cl)
	if err != nil {
		t.Errorf("Unable to get revision spec: %s", err)
	}
}

func TestBuildEntry(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	cl, err := s.OpenChangeLog()
	if err != nil {
		t.Fatalf("Unable to open change log: %s", err)
	}

	rec, err := hg_revlog.NodeIdRevSpec(revision).Lookup(cl)
	if err != nil {
		t.Errorf("Unable to get revision spec: %s", err)
	}

	fb := hg_revlog.NewFileBuilder()
	_, err = hg_changelog.BuildEntry(rec, fb)
	if err != nil {
		t.Errorf("Unable to build entry: %s", err)
	}
}

func TestBuildManifest(t *testing.T) {
	defer removeTmpDirs(t)

	dir := createRepo(t, commands)

	repo, err := hgo.OpenRepository(dir)
	if err != nil {
		t.Fatal("Unable to open repository.")
	}

	s := repo.NewStore()
	if s == nil {
		t.Fatal("Unable to create new store")
	}

	cl, err := s.OpenChangeLog()
	if err != nil {
		t.Fatalf("Unable to open change log: %s", err)
	}

	rec, err := hg_revlog.NodeIdRevSpec(revision).Lookup(cl)
	if err != nil {
		t.Errorf("Unable to get revision spec: %s", err)
	}

	fb := hg_revlog.NewFileBuilder()
	ce, err := hg_changelog.BuildEntry(rec, fb)
	if err != nil {
		t.Errorf("Unable to build entry: %s", err)
	}

	mlog, err := s.OpenManifests()
	if err != nil {
		t.Errorf("Unable to open manifest: %s", err)
	}

	rec2, err := mlog.LookupRevision(int(ce.Linkrev), ce.ManifestNode)
	if err != nil {
		t.Errorf("Unable to lookup revision: %s", err)
	}

	_, err = hg_store.BuildManifest(rec2, fb)
	if err != nil {
		t.Errorf("Unable to build manifest: %s", err)
	}
}
