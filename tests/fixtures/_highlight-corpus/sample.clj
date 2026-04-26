;; Clojure sample
(ns sample.core
  (:require [clojure.java.io :as io]
            [clojure.string :as str])
  (:gen-class))

(defrecord Counter [value])

(defn increment
  ([counter] (increment counter 1))
  ([{:keys [value] :as counter} n]
   (assoc counter :value (+ value n))))

(defn count-bytes [path]
  (with-open [r (io/reader path)]
    (->> (line-seq r)
         (remove str/blank?)
         (reduce (fn [c line] (increment c (count line)))
                 (->Counter 0))
         :value)))

(defn -main [& [path]]
  (let [path  (or path "input.txt")
        total (count-bytes path)]
    (println (format "total bytes: %d" total))))
