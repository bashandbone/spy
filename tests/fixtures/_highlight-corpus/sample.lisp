;;; Common Lisp sample
(defpackage :sample
  (:use :cl)
  (:export #:count-bytes #:main))

(in-package :sample)

(defclass counter ()
  ((value :initform 0 :accessor counter-value)))

(defmethod increment ((c counter) n)
  (incf (counter-value c) n)
  (counter-value c))

(defun count-bytes (path)
  (let ((counter (make-instance 'counter)))
    (with-open-file (stream path :direction :input)
      (loop for line = (read-line stream nil nil)
            while line
            unless (zerop (length line))
              do (increment counter (length line))))
    (counter-value counter)))

(defun main (&optional (path "input.txt"))
  (let ((total (count-bytes path)))
    (format t "total bytes: ~D~%" total)
    total))
