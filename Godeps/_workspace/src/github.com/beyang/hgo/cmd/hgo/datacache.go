package main

type dataCache []dataEnt

type dataEnt struct {
	rev  int
	data []byte
}

var cacheStats int

func (c dataCache) Get(rev int) []byte {
	for i := range c {
		if c[i].rev == rev {
			return c[i].data
		}
	}
	return nil
}

func (pc *dataCache) Store(rev int, data []byte) {
	var p, maxp *dataEnt
	var maxrev int

	c := *pc
	n := len(c)

	for i := range c {
		p = &c[i]
		if p.rev == rev {
			return
		}
		if p.rev > maxrev {
			maxrev = p.rev
			maxp = p
		}
	}

	e := dataEnt{rev, data}

	if n < cap(c) {
		c = c[:n+1]
		c[n] = e
		*pc = c
	} else {
		*maxp = e
	}

}

var dc = make(dataCache, 0, 4)
