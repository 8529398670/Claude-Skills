#!/bin/bash
ssh-add -D
git init
git config --global --unset user.name
git config --global --unset user.email
git config user.name "8529398670"
git config user.email "banjo.kazooie.2955@gmail.com"
ssh-add -k /home/sd/.ssh/github-8529398670

# Count on from the highest numbered commit, so a worded message can't reset it.
LastCommit=$(git log --pretty=%s | grep -E '^[0-9]+$' | sort -n | tail -1)
NextCommitNumber=$((${LastCommit:-0}+1))

git add .
git tag -l | xargs git tag -d
# With nothing to commit there is no new number, so no tag is made or moved.
if git diff --cached --quiet; then
	echo "Nothing new to commit, pushing what is already here"
elif [ -n "$1" ]; then
	git commit -m "$1"
	git tag v1.0.$1
else
	git commit -m "$NextCommitNumber"
	git tag v1.0.$NextCommitNumber
fi
git remote add origin git@github-8529398670:8529398670/Claude-Skills.git
git push origin --tags --force
git push origin master
