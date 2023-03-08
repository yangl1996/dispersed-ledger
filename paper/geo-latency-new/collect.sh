for f in ./*dl*.csv; do
	cat $f | head -3 | tail -1
done
