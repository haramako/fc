# fuzz の種

`internal/syntax` の FuzzParse / FuzzFormat と `internal/driver` の FuzzCheck が、リポジトリの `.fc`（test・fclib・examples）と
一緒に種として読む。版ごとの新しい文法（fc 3: for-each・範囲・case の範囲・`..=`・@min、fc 4: `+%`・`@printf`・`else if`・
文字列のエスケープ・文字のリテラル・型を書かない文字列の配列など）で、リポジトリの `.fc` にはまだ少ない形を置く。
