" Vim Script sample
if exists('g:loaded_sample')
    finish
endif
let g:loaded_sample = 1

let s:counter = 0

function! s:Increment(n) abort
    let s:counter += a:n
    return s:counter
endfunction

function! sample#count(path) abort
    let l:total = 0
    for l:line in readfile(a:path)
        if !empty(l:line)
            let l:total = s:Increment(strlen(l:line))
        endif
    endfor
    echom printf('total bytes: %d', l:total)
    return l:total
endfunction

command! -nargs=1 SampleCount call sample#count(<f-args>)

augroup sample_aug
    autocmd!
    autocmd BufWritePost *.txt call sample#count(expand('<afile>'))
augroup END
