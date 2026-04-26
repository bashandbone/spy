%% Erlang sample
-module(sample).
-export([main/1, count_bytes/1]).

-record(counter, {value = 0 :: integer()}).

increment(#counter{value = V}, N) ->
    #counter{value = V + N}.

count_bytes(Path) ->
    {ok, Bin} = file:read_file(Path),
    Lines = binary:split(Bin, <<"\n">>, [global]),
    lists:foldl(fun count_line/2, #counter{}, Lines).

count_line(<<>>, Acc) ->
    Acc;
count_line(Line, Acc) ->
    increment(Acc, byte_size(Line)).

main([]) ->
    main(["input.txt"]);
main([Path | _]) ->
    Counter = count_bytes(Path),
    io:format("total bytes: ~p~n", [Counter#counter.value]).
