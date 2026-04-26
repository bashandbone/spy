# R sample
read_lines <- function(path) {
  con <- file(path, "r")
  on.exit(close(con))
  readLines(con)
}

count_bytes <- function(lines) {
  total <- 0L
  for (line in lines) {
    if (nchar(line) > 0L) {
      total <- total + nchar(line)
    }
  }
  total
}

main <- function(argv) {
  path <- if (length(argv) > 0) argv[1] else "input.txt"
  if (!file.exists(path)) {
    message("input not found: ", path)
    return(invisible(1L))
  }
  lines <- read_lines(path)
  total <- count_bytes(lines)
  cat(sprintf("total bytes: %d\n", total))
  invisible(0L)
}

if (!interactive()) {
  args <- commandArgs(trailingOnly = TRUE)
  status <- main(args)
  quit(status = status)
}
