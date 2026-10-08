function gwt --description "Git Worktree TUI Manager"
    set target_dir (gwt-bin)
    if test -n "$target_dir"
        if test -d "$target_dir"
            cd "$target_dir"
        else
            echo "Error: Directory does not exist -> $target_dir"
        end
    end
end
