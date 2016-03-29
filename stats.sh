# Print Git commit statistics for a specific author
# Usage: git-stats "Linus Torvalds"
git-stats() {
	author=${1-`git config --get user.name`}

	echo "Commit stats for \033[1;37m$author\033[0m:"
	git log --shortstat --author $author -i 2> /dev/null \
		| grep -E 'files? changed' \
		| awk 'BEGIN{commits=0;inserted=0;deleted=0} \
			{commits+=1; if($5!~"^insertion") { deleted+=$4 } \
			else { inserted+=$4; deleted+=$6 } } END \
			{print "\033[1;34m↑↑\033[1;37m", commits \
			"\n\033[1;32m++\033[1;37m", inserted, \
			"\n\033[1;31m--\033[1;37m", deleted, "\033[0m"}'
}
