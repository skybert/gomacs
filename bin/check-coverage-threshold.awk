/^total:/ {
    gsub(/%/, "", $3)
    total = $3 + 0
    if (total < min + 0) {
        printf "FAIL: %.1f%% < %d%%\n", total, min
        exit 1
    }
    printf "OK: %.1f%% >= %d%%\n", total, min
}
