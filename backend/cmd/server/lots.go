package main

// З10-и2 (OWNER.md, трек З; №68, №69, №70): партии склада — распределитель.
// Чистые функции без базы: из каких партий списать (FIFO), куда вернуть
// отменённое (в те же партии), как разбить строку поставки на несколько цен и
// перенести уже списанное. Списание и возврат подключает З10-и3 (stockMove),
// разбивку — З10-и5 (цены поставки). Таблицы — миграция 076.

import (
	"errors"

	"github.com/google/uuid"
)

// lotLeft — партия для списания: id и сколько в ней осталось. Порядок FIFO
// (received_at, part) задаёт вызывающий.
type lotLeft struct {
	ID   uuid.UUID
	Left int
}

// lotPart — из какой партии (или в какую) сколько штук, без знака.
type lotPart struct {
	ID  uuid.UUID
	Qty int
}

var (
	errLotsShort = errors.New("lots_short") // в партиях меньше, чем списываем: остаток и партии разошлись
	errLotsBack  = errors.New("lots_back")  // возвращают больше, чем это движение взяло
	errLotSplit  = errors.New("lot_split")  // разбивка не сходится с движениями строки
)

// lotFIFO — списание qty штук: с первой партии, где что-то осталось, дальше —
// по порядку; пустые пропускаются. qty ≤ 0 — ничего. Не хватило — errLotsShort.
// Чистая, тест.
func lotFIFO(lots []lotLeft, qty int) ([]lotPart, error) {
	out := []lotPart{}
	for _, l := range lots {
		if qty <= 0 {
			break
		}
		if l.Left <= 0 {
			continue
		}
		n := min(l.Left, qty)
		out = append(out, lotPart{l.ID, n})
		qty -= n
	}
	if qty > 0 {
		return nil, errLotsShort
	}
	return out, nil
}

// lotBack — возврат qty штук отменённого движения в те же партии: taken —
// откуда оно взяло (по порядку списания), returned — что уже вернули раньше
// (частичные отмены). Возвращает в обратном порядке: последняя тронутая партия —
// первой. Больше, чем осталось невозвращённым, — errLotsBack. Чистая, тест.
func lotBack(taken, returned []lotPart, qty int) ([]lotPart, error) {
	nets, idx := []lotPart{}, map[uuid.UUID]int{}
	for _, t := range taken {
		if i, ok := idx[t.ID]; ok {
			nets[i].Qty += t.Qty
			continue
		}
		idx[t.ID] = len(nets)
		nets = append(nets, t)
	}
	for _, r := range returned {
		i, ok := idx[r.ID]
		if !ok || nets[i].Qty < r.Qty {
			return nil, errLotsBack // вернули туда, откуда не брали, или больше взятого
		}
		nets[i].Qty -= r.Qty
	}
	out := []lotPart{}
	for i := len(nets) - 1; i >= 0 && qty > 0; i-- {
		if nets[i].Qty <= 0 {
			continue
		}
		n := min(nets[i].Qty, qty)
		out = append(out, lotPart{nets[i].ID, n})
		qty -= n
	}
	if qty > 0 {
		return nil, errLotsBack
	}
	return out, nil
}

// lotPrice — кусок строки поставки при оценке: сколько штук по какой цене
// (гроши за штуку).
type lotPrice struct {
	Qty   int
	Price int64
}

// lotEntry — движение строки поставки (всех её кусков, суммой по движению, по
// порядку времени): In — приход, создавший строку; Qty < 0 — списание; Qty > 0
// без In — возврат, Source — какое списание он отменяет (nil или списание не
// из этой строки — возврат продажи, сделанной до партий).
type lotEntry struct {
	Move   uuid.UUID
	Qty    int
	In     bool
	Source *uuid.UUID
}

// lotPiece — кусок строки после разбивки: цена, пришло, осталось.
type lotPiece struct {
	Price int64
	In    int
	Left  int
}

// lotAlloc — доля движения в куске: номер куска (как в prices) и сколько штук
// (знак — как у движения).
type lotAlloc struct {
	Piece int
	Qty   int
}

// lotSplit — разбивка строки поставки на цены с переносом уже списанного
// (З10-и5, «20 × 2,50 + 10 × 2,80»): prices — куски по порядку, их штуки в
// сумме — сколько пришло по строке; entries — её движения по порядку. Приход
// раскладывается по кускам их количествами, по порядку; списания — FIFO по
// кускам (проданное — по первой цене); возврат — в те куски, откуда взяло его
// списание (последний тронутый — первым), без источника — в первый кусок.
// Повторная оценка — та же разбивка всех движений строки заново. Возвращает
// куски с остатками и долю каждого движения по кускам (allocs[i] — для
// entries[i]). Цена меньше нуля, кусок без штук, «пришло» не сходится с
// кусками, списание больше, чем было в кусках, — errLotSplit. Чистая, тест.
func lotSplit(prices []lotPrice, entries []lotEntry) ([]lotPiece, [][]lotAlloc, error) {
	if len(prices) == 0 {
		return nil, nil, errLotSplit
	}
	pieces, total := make([]lotPiece, len(prices)), 0
	for i, p := range prices {
		if p.Qty <= 0 || p.Price < 0 {
			return nil, nil, errLotSplit
		}
		pieces[i] = lotPiece{Price: p.Price, In: p.Qty}
		total += p.Qty
	}
	in := 0
	for _, e := range entries {
		if e.In {
			if e.Qty <= 0 {
				return nil, nil, errLotSplit
			}
			in += e.Qty
		}
	}
	if in != total {
		return nil, nil, errLotSplit // куски не сходятся с «пришло»
	}
	allocs := make([][]lotAlloc, len(entries))
	filled := make([]int, len(pieces)) // сколько прихода уже разложено по куску
	took := map[uuid.UUID][]lotAlloc{} // списание → из каких кусков взяло (невозвращённое)
	for i, e := range entries {
		switch {
		case e.In: // приход — по кускам их количествами, по порядку
			q := e.Qty
			for k := range pieces {
				if q == 0 {
					break
				}
				n := min(pieces[k].In-filled[k], q)
				if n <= 0 {
					continue
				}
				filled[k] += n
				pieces[k].Left += n
				allocs[i] = append(allocs[i], lotAlloc{k, n})
				q -= n
			}
		case e.Qty < 0: // списание — FIFO по кускам
			q := -e.Qty
			for k := range pieces {
				if q == 0 {
					break
				}
				n := min(pieces[k].Left, q)
				if n <= 0 {
					continue
				}
				pieces[k].Left -= n
				allocs[i] = append(allocs[i], lotAlloc{k, -n})
				took[e.Move] = append(took[e.Move], lotAlloc{k, n})
				q -= n
			}
			if q > 0 {
				return nil, nil, errLotSplit // списали больше, чем было в кусках
			}
		case e.Qty > 0: // возврат — туда, откуда взяло его списание
			q := e.Qty
			if e.Source != nil {
				src := took[*e.Source]
				for j := len(src) - 1; j >= 0 && q > 0; j-- {
					n := min(src[j].Qty, q)
					if n <= 0 {
						continue
					}
					src[j].Qty -= n
					pieces[src[j].Piece].Left += n
					allocs[i] = append(allocs[i], lotAlloc{src[j].Piece, n})
					q -= n
				}
			}
			if q > 0 { // без источника (или вернули больше, чем взяли из строки) — в первый кусок
				pieces[0].Left += q
				allocs[i] = addAlloc(allocs[i], 0, q)
			}
		}
	}
	return pieces, allocs, nil
}

// addAlloc — прибавить к доле движения в куске (одна строка на кусок).
func addAlloc(a []lotAlloc, piece, qty int) []lotAlloc {
	for i := range a {
		if a[i].Piece == piece {
			a[i].Qty += qty
			return a
		}
	}
	return append(a, lotAlloc{piece, qty})
}
