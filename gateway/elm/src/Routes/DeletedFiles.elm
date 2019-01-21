module Routes.DeletedFiles exposing (Model, Msg, newModel, subscriptions, update, view)

import Browser.Navigation as Nav
import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import Html.Lazy as Lazy
import Time



-- MODEL:


type alias Model =
    { key : Nav.Key
    , zone : Time.Zone
    }


newModel : Nav.Key -> Time.Zone -> Model
newModel key zone =
    Model key zone



-- MESSAGES:


type Msg
    = Bla



-- UPDATE:


update : Msg -> Model -> ( Model, Cmd Msg )
update msg model =
    ( model, Cmd.none )



-- VIEW:


view : Model -> Html Msg
view model =
    text "Here you will see a list of deleted files with the option to recover them."



-- SUBSCRIPTIONS:


subscriptions : Model -> Sub Msg
subscriptions model =
    Sub.none