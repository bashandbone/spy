-- Elm sample
module Sample exposing (main)

import Browser
import Html exposing (Html, button, div, h1, text)
import Html.Attributes exposing (class)
import Html.Events exposing (onClick)


type alias Model =
    { count : Int }


type Msg
    = Increment
    | Decrement


init : Model
init =
    { count = 0 }


update : Msg -> Model -> Model
update msg model =
    case msg of
        Increment ->
            { model | count = model.count + 1 }

        Decrement ->
            { model | count = model.count - 1 }


view : Model -> Html Msg
view model =
    div [ class "counter" ]
        [ h1 [] [ text "spy demo" ]
        , div [] [ text (String.fromInt model.count) ]
        , button [ onClick Decrement ] [ text "-" ]
        , button [ onClick Increment ] [ text "+" ]
        ]


main : Program () Model Msg
main =
    Browser.sandbox { init = init, update = update, view = view }
