-- Haskell sample
{-# LANGUAGE OverloadedStrings #-}

module Main (main) where

import qualified Data.Text as T
import qualified Data.Text.IO as TIO
import System.IO (hPutStrLn, stderr)
import System.Exit (exitFailure)
import Control.Exception (try, SomeException)

newtype Counter = Counter { unCounter :: Int }
  deriving (Show)

increment :: Int -> Counter -> Counter
increment n (Counter v) = Counter (v + n)

countBytes :: T.Text -> Counter
countBytes = foldr step (Counter 0) . filter (not . T.null) . T.lines
  where
    step line acc = increment (T.length line) acc

main :: IO ()
main = do
  result <- try (TIO.readFile "input.txt") :: IO (Either SomeException T.Text)
  case result of
    Left err -> hPutStrLn stderr (show err) >> exitFailure
    Right txt -> do
      let total = unCounter (countBytes txt)
      putStrLn ("total bytes: " ++ show total)
