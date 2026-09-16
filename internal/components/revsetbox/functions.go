package revsetbox

// FuncSpec documents one revset function or built-in alias for the signature-help popup.
type FuncSpec struct {
	Name      string
	Signature string
	Blurb     string
}

// builtinFunctions is the completion and signature-help fallback, recorded against jj
// 0.45.1. `git_head` and `git_refs` are unverified: that jj version rejects both.
var builtinFunctions = []FuncSpec{
	{"all", "all()", "All visible commits and ancestors of commits explicitly mentioned"},
	{"ancestors", "ancestors(x[, depth])", "Ancestors of x, optionally limited to depth"},
	{"author", "author(pattern)", "Commits whose author name or email matches pattern"},
	{"author_date", "author_date(pattern)", "Commits whose author date matches the date pattern"},
	{"author_email", "author_email(pattern)", "Commits whose author email matches pattern"},
	{"author_name", "author_name(pattern)", "Commits whose author name matches pattern"},
	{"bisect", "bisect(x)", "Commits for which about half of x are descendants"},
	{"bookmarks", "bookmarks([pattern])", "Local bookmark targets, optionally matching pattern"},
	{"builtin_immutable_heads", "builtin_immutable_heads()", "Default definition backing immutable_heads()"},
	{"builtin_log", "builtin_log()", "Default definition backing revsets.log"},
	{"change_id", "change_id(prefix)", "Commits with the given change ID prefix"},
	{"children", "children(x[, depth])", "Children of x, optionally at the given depth"},
	{"coalesce", "coalesce(revsets...)", "First revset in the list that isn't none()"},
	{"commit_id", "commit_id(prefix)", "Commits with the given commit ID prefix"},
	{"committer", "committer(pattern)", "Commits whose committer name or email matches pattern"},
	{"committer_date", "committer_date(pattern)", "Commits whose committer date matches the date pattern"},
	{"committer_email", "committer_email(pattern)", "Commits whose committer email matches pattern"},
	{"committer_name", "committer_name(pattern)", "Commits whose committer name matches pattern"},
	{"conflicts", "conflicts()", "Commits that have files in a conflicted state"},
	{"connected", "connected(x)", "Same as x::x; useful when x has several commits"},
	{"description", "description(pattern)", "Commits whose description matches pattern"},
	{
		"diff_contains", "diff_contains(text[, files])",
		"Deprecated: use diff_lines(). Commits whose diff contains text",
	},
	{"diff_lines", "diff_lines(text[, files])", "Commits whose diff matches text line by line"},
	{"diff_lines_added", "diff_lines_added(text[, files])", "diff_lines(), matching only added lines"},
	{"diff_lines_removed", "diff_lines_removed(text[, files])", "diff_lines(), matching only removed lines"},
	{"divergent", "divergent()", "Commits that are divergent"},
	{"empty", "empty()", "Commits modifying no files, including merges() and root()"},
	{"exactly", "exactly(x, count)", "x, erroring unless it is exactly size count"},
	{"files", "files(expression)", "Commits modifying paths matching the fileset expression"},
	{"first_ancestors", "first_ancestors(x[, depth])", "ancestors(), following only the first parent"},
	{"first_parent", "first_parent(x[, depth])", "parents(), returning only the first parent of a merge"},
	{"fork_point", "fork_point(x)", "Common ancestor(s) of all commits in x"},
	{"forks", "forks()", "Fork commits, those with more than one child"},
	{"git_head", "git_head()", "The commit checked out by git HEAD in a colocated repo"},
	{"git_refs", "git_refs()", "Commits pointed to by git refs"},
	{"heads", "heads(x)", "Commits in x that are not ancestors of other commits in x"},
	{"hidden", "hidden()", "The set of hidden commits; ~visible()"},
	{"immutable", "immutable()", "The set of commits jj treats as immutable"},
	{"immutable_heads", "immutable_heads()", "The heads of the set of immutable commits"},
	{"latest", "latest(x[, count])", "The latest count commits in x by committer timestamp"},
	{"merge_point", "merge_point(x)", "Common descendant(s) of all commits in x"},
	{"merges", "merges()", "Merge commits"},
	{"mine", "mine()", "Commits authored by the current user"},
	{"mutable", "mutable()", "The set of commits jj treats as mutable; ~immutable()"},
	{"none", "none()", "No commits"},
	{"parents", "parents(x[, depth])", "Parents of x, optionally at the given depth"},
	{"present", "present(x)", "x, evaluated to none() if any commit in x doesn't exist"},
	{"reachable", "reachable(srcs, domain)", "Commits reachable from srcs without leaving domain"},
	{"remote_bookmarks", "remote_bookmarks([name], [remote])", "Remote bookmark targets matching name and remote"},
	{"remote_tags", "remote_tags([name], [remote])", "Remote tag targets matching name and remote"},
	{"roots", "roots(x)", "Commits in x that are not descendants of other commits in x"},
	{"root", "root()", "The virtual commit that is the ancestor of all other commits"},
	{"signed", "signed()", "Commits that are cryptographically signed"},
	{"subject", "subject(pattern)", "Commits whose description's first line matches pattern"},
	{"tags", "tags([pattern])", "Tag targets, optionally matching pattern"},
	{"tracked_remote_bookmarks", "tracked_remote_bookmarks([name], [remote])", "Targets of tracked remote bookmarks"},
	{"tracked_remote_tags", "tracked_remote_tags([name], [remote])", "Targets of tracked remote tags"},
	{"trunk", "trunk()", "The head commit of the repo's default bookmark and remote"},
	{
		"untracked_remote_bookmarks", "untracked_remote_bookmarks([name], [remote])",
		"Targets of untracked remote bookmarks",
	},
	{"untracked_remote_tags", "untracked_remote_tags([name], [remote])", "Targets of untracked remote tags"},
	{"visible", "visible()", "The set of visible commits; ::visible_heads()"},
	{"visible_heads", "visible_heads()", "All visible heads"},
	{"working_copies", "working_copies()", "The working copy commits across all workspaces"},
}

var builtinNames = func() []string {
	names := make([]string, len(builtinFunctions))
	for i, f := range builtinFunctions {
		names[i] = f.Name
	}

	return names
}()

var builtinByName = func() map[string]FuncSpec {
	byName := make(map[string]FuncSpec, len(builtinFunctions))
	for _, f := range builtinFunctions {
		byName[f.Name] = f
	}

	return byName
}()
