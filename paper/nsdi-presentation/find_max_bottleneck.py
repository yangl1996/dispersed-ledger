uk = open("concurrent-uk.trace")
dutch = open("concurrent-dutch.trace")
singapore = open("concurrent-singapore.trace")
brazil = open("concurrent-brazil.trace")
indonesia = open("concurrent-indonesia.trace")

ds = [[], [], [], [], []]

idx = 0
mlen = 100000
for file in [uk, singapore, brazil, indonesia, dutch]:
    for line in file:
        l = line.strip()
        if "sender" in l:
            continue
        if "receiver" in l:
            continue
        if l == "":
            continue
        ds[idx].append(float(l))
    if mlen > len(ds[idx]):
        mlen = len(ds[idx])
    idx += 1

max_tot = 0.0
min_tot = 0.0
for t in range(mlen):
    bw = []
    for i in range(4):
        bw.append(ds[i][t])
    bw.sort()
    print(bw[1], "  ", bw[3])
    max_tot += bw[3]
    min_tot += bw[1]

print("# ", max_tot/min_tot)
