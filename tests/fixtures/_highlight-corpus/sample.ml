(* OCaml sample *)
module Counter = struct
  type t = { mutable value : int }

  let make () = { value = 0 }

  let increment c n =
    c.value <- c.value + n;
    c.value

  let value c = c.value
end

let read_lines path =
  let ic = open_in path in
  let lines = ref [] in
  try
    while true do
      lines := input_line ic :: !lines
    done;
    assert false
  with End_of_file ->
    close_in ic;
    List.rev !lines

let () =
  let counter = Counter.make () in
  let lines = read_lines "input.txt" in
  List.iter
    (fun line ->
      if String.length line > 0 then
        ignore (Counter.increment counter (String.length line)))
    lines;
  Printf.printf "total bytes: %d\n" (Counter.value counter)
