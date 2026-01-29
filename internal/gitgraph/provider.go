package gitgraph

import (
	"container/heap"
	"fmt"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type GraphCell struct {
	Ch    string
	Color int
}

type CommitInfo struct {
	Hash      plumbing.Hash
	ShortHash string
	Subject   string
	Author    string
	When      time.Time
	Graph     []GraphCell
	Commit    *object.Commit
}

type CommitProvider struct {
	repo     *git.Repository
	all      bool
	limit    int
	seen     map[plumbing.Hash]bool
	heap     commitHeap
	graph    graphState
	Commits  []*CommitInfo
	complete bool
}

func NewCommitProvider(repo *git.Repository, includeAll bool, limit int) (*CommitProvider, error) {
	p := &CommitProvider{
		repo:  repo,
		all:   includeAll,
		limit: limit,
		seen:  make(map[plumbing.Hash]bool),
	}

	tips, err := gatherTips(repo, includeAll)
	if err != nil {
		return nil, err
	}
	if len(tips) == 0 {
		return nil, fmt.Errorf("no commits found")
	}
	for _, h := range tips {
		if p.seen[h] {
			continue
		}
		commit, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		p.seen[h] = true
		heap.Push(&p.heap, commit)
	}
	return p, nil
}

func (p *CommitProvider) HasMore() bool {
	if p.limit > 0 && len(p.Commits) >= p.limit {
		return false
	}
	return p.heap.Len() > 0
}

func (p *CommitProvider) Ensure(index int) error {
	if index < 0 {
		return nil
	}
	for len(p.Commits) <= index && p.HasMore() {
		if err := p.loadNext(); err != nil {
			return err
		}
	}
	if p.heap.Len() == 0 && (p.limit == 0 || len(p.Commits) < p.limit) {
		p.complete = true
	}
	return nil
}

func (p *CommitProvider) loadNext() error {
	commit := heap.Pop(&p.heap).(*object.Commit)
	info := buildCommitInfo(commit, &p.graph)
	p.Commits = append(p.Commits, info)

	if p.limit > 0 && len(p.Commits) >= p.limit {
		return nil
	}

	for _, parent := range commit.ParentHashes {
		if p.seen[parent] {
			continue
		}
		parentCommit, err := p.repo.CommitObject(parent)
		if err != nil {
			continue
		}
		p.seen[parent] = true
		heap.Push(&p.heap, parentCommit)
	}
	return nil
}

func gatherTips(repo *git.Repository, includeAll bool) ([]plumbing.Hash, error) {
	var tips []plumbing.Hash
	iter, err := repo.References()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	_ = iter.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name()
		if !includeAll {
			if !name.IsBranch() && name != plumbing.HEAD {
				return nil
			}
		} else {
			if !name.IsBranch() && !name.IsRemote() && name != plumbing.HEAD {
				return nil
			}
		}
		if ref.Type() != plumbing.HashReference {
			return nil
		}
		tips = append(tips, ref.Hash())
		return nil
	})
	if len(tips) == 0 {
		if head, err := repo.Head(); err == nil {
			tips = append(tips, head.Hash())
		}
	}
	return tips, nil
}

func buildCommitInfo(commit *object.Commit, graph *graphState) *CommitInfo {
	subject := firstLine(commit.Message)
	cells := graph.Render(commit)
	return &CommitInfo{
		Hash:      commit.Hash,
		ShortHash: commit.Hash.String()[:7],
		Subject:   subject,
		Author:    commit.Author.Name,
		When:      commit.Committer.When,
		Graph:     cells,
		Commit:    commit,
	}
}

func firstLine(message string) string {
	parts := strings.SplitN(message, "\n", 2)
	return strings.TrimSpace(parts[0])
}

type graphState struct {
	columns []plumbing.Hash
}

func (g *graphState) Render(commit *object.Commit) []GraphCell {
	idx := indexOfHash(g.columns, commit.Hash)
	if idx == -1 {
		g.columns = append([]plumbing.Hash{commit.Hash}, g.columns...)
		idx = 0
	}
	parents := commit.ParentHashes
	preLen := len(g.columns)
	postLen := preLen
	if len(parents) > 1 {
		postLen = preLen + (len(parents) - 1)
	}
	cells := make([]GraphCell, postLen)
	for i := 0; i < postLen; i++ {
		cells[i] = GraphCell{Ch: "|", Color: i}
	}
	if idx < len(cells) {
		cells[idx].Ch = "*"
	}
	if len(parents) > 1 {
		for i := 1; i < len(parents); i++ {
			pos := idx + i
			if pos < len(cells) {
				cells[pos].Ch = "\\"
			}
		}
	}

	if len(parents) == 0 {
		g.columns = append(g.columns[:idx], g.columns[idx+1:]...)
	} else {
		g.columns[idx] = parents[0]
		for i := 1; i < len(parents); i++ {
			insertAt := idx + i
			g.columns = append(g.columns[:insertAt], append([]plumbing.Hash{parents[i]}, g.columns[insertAt:]...)...)
		}
	}
	g.columns = dedupeHashes(g.columns)
	return cells
}

func indexOfHash(list []plumbing.Hash, target plumbing.Hash) int {
	for i, h := range list {
		if h == target {
			return i
		}
	}
	return -1
}

func dedupeHashes(list []plumbing.Hash) []plumbing.Hash {
	seen := make(map[plumbing.Hash]bool, len(list))
	out := make([]plumbing.Hash, 0, len(list))
	for _, h := range list {
		if seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}

type commitHeap []*object.Commit

func (h commitHeap) Len() int { return len(h) }
func (h commitHeap) Less(i, j int) bool {
	if h[i].Committer.When.Equal(h[j].Committer.When) {
		return h[i].Hash.String() > h[j].Hash.String()
	}
	return h[i].Committer.When.After(h[j].Committer.When)
}
func (h commitHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *commitHeap) Push(x interface{}) {
	*h = append(*h, x.(*object.Commit))
}
func (h *commitHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}
