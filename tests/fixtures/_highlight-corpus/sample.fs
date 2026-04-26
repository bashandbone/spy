// F# sample
module Sample

open System.IO

type Counter = { Value: int } with
    member this.Increment n = { Value = this.Value + n }

let countBytes (path: string) : int =
    if not (File.Exists path) then
        eprintfn "input not found: %s" path
        0
    else
        File.ReadLines(path)
        |> Seq.filter (fun line -> line.Length > 0)
        |> Seq.fold (fun (c: Counter) (line: string) -> c.Increment line.Length) { Value = 0 }
        |> fun c -> c.Value

[<EntryPoint>]
let main argv =
    let path = if argv.Length > 0 then argv.[0] else "input.txt"
    let total = countBytes path
    printfn "total bytes: %d" total
    0
