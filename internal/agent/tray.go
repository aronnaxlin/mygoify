package agent

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// The agent's icon in the tray, on desktops other than Omarchy, whose bar
// shows a widget of its own instead (DESIGN.md §4.3): a StatusNotifierItem,
// which a left click activates to open the mini player, and whose menu,
// a dbusmenu, opens the app or quits.
const (
	itemPath   = "/StatusNotifierItem"
	itemIface  = "org.kde.StatusNotifierItem"
	menuPath   = "/MenuBar"
	menuIface  = "com.canonical.dbusmenu"
	watcher    = "org.kde.StatusNotifierWatcher"
	watcherObj = "/StatusNotifierWatcher"
)

// TrayItem is what a tray icon shows, and does as it is used.
type TrayItem struct {
	Title string
	Icon  []byte // a PNG
	// Activate is a left click, as the mini player opens.
	Activate func()
	// Items are its menu's, in order, with "" for a line between them.
	Items []MenuItem
}

// MenuItem is an item of the tray icon's menu.
type MenuItem struct {
	Label  string
	Chosen func()
}

// sni is the tray icon on the session bus.
type sni struct {
	item TrayItem
	conn *dbus.Conn
	name string
}

// exportTray puts the tray icon on the bus, and registers it with the
// desktop's tray, again whenever the tray starts anew.
func exportTray(conn *dbus.Conn, item TrayItem) error {
	s := &sni{item: item, conn: conn, name: fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())}
	pixmaps, err := pixmap(item.Icon)
	if err != nil {
		return err
	}
	if err := conn.Export(s, itemPath, itemIface); err != nil {
		return err
	}
	props, err := prop.Export(conn, itemPath, prop.Map{itemIface: {
		"Category":   {Value: "ApplicationStatus"},
		"Id":         {Value: "mygoify"},
		"Title":      {Value: item.Title},
		"Status":     {Value: "Active"},
		"IconName":   {Value: ""},
		"IconPixmap": {Value: pixmaps},
		"ToolTip":    {Value: tooltip{Title: item.Title, Icon: pixmaps}},
		"ItemIsMenu": {Value: false},
		"Menu":       {Value: dbus.ObjectPath(menuPath)},
	}})
	if err != nil {
		return err
	}
	if err := conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: itemPath,
		Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData,
			{Name: itemIface, Methods: introspect.Methods(s), Properties: props.Introspection(itemIface)}},
	}), itemPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}
	m := &menu{items: item.Items}
	if err := conn.Export(m, menuPath, menuIface); err != nil {
		return err
	}
	mprops, err := prop.Export(conn, menuPath, prop.Map{menuIface: {
		"Version":       {Value: uint32(3)},
		"TextDirection": {Value: "ltr"},
		"Status":        {Value: "normal"},
		"IconThemePath": {Value: []string{}},
	}})
	if err != nil {
		return err
	}
	if err := conn.Export(introspect.NewIntrospectable(&introspect.Node{
		Name: menuPath,
		Interfaces: []introspect.Interface{introspect.IntrospectData, prop.IntrospectData,
			{Name: menuIface, Methods: introspect.Methods(m), Properties: mprops.Introspection(menuIface)}},
	}), menuPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		return err
	}
	if _, err := conn.RequestName(s.name, dbus.NameFlagDoNotQueue); err != nil {
		return err
	}
	// The tray may start after the agent, or again: register each time.
	if err := conn.AddMatchSignal(dbus.WithMatchInterface("org.freedesktop.DBus"), dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, watcher)); err != nil {
		return err
	}
	signals := make(chan *dbus.Signal, 8)
	conn.Signal(signals)
	go func() {
		for sig := range signals {
			if sig.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(sig.Body) == 3 && sig.Body[2] != "" {
				s.register()
			}
		}
	}()
	s.register()
	return nil
}

// register tells the desktop's tray of the icon, if there is one.
func (s *sni) register() {
	call := s.conn.Object(watcher, watcherObj).Call(watcher+".RegisterStatusNotifierItem", 0, s.name)
	if call.Err != nil {
		log.Printf("agent: tray: %v", call.Err)
	}
}

// Activate is a left click on the icon.
func (s *sni) Activate(x, y int32) *dbus.Error {
	if s.item.Activate != nil {
		go s.item.Activate()
	}
	return nil
}

// SecondaryActivate is a middle click: as a left one.
func (s *sni) SecondaryActivate(x, y int32) *dbus.Error { return s.Activate(x, y) }

// ContextMenu does nothing: the tray shows Menu.
func (s *sni) ContextMenu(x, y int32) *dbus.Error { return nil }

func (s *sni) Scroll(delta int32, orientation string) *dbus.Error { return nil }

// tooltip is StatusNotifierItem's ToolTip, (sa(iiay)ss).
type tooltip struct {
	IconName string
	Icon     []pixels
	Title    string
	Text     string
}

// pixels is an icon at one size, (iiay): ARGB32 in network order.
type pixels struct {
	W, H int32
	Data []byte
}

// pixmap is the PNG as StatusNotifierItem's IconPixmap.
func pixmap(b []byte) ([]pixels, error) {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	r := img.Bounds()
	rgba := image.NewNRGBA(r)
	draw.Draw(rgba, r, img, r.Min, draw.Src)
	data := make([]byte, 0, r.Dx()*r.Dy()*4)
	for i := 0; i < len(rgba.Pix); i += 4 {
		data = append(data, rgba.Pix[i+3], rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2])
	}
	return []pixels{{W: int32(r.Dx()), H: int32(r.Dy()), Data: data}}, nil
}

// menu is the icon's menu, as dbusmenu has hosts show it: a root, 0, and
// its items, 1 and on.
type menu struct {
	mu       sync.Mutex
	items    []MenuItem
	revision uint32
}

// layout is a dbusmenu item, (ia{sv}av).
type layout struct {
	ID       int32
	Props    map[string]dbus.Variant
	Children []dbus.Variant
}

func (m *menu) node(id int32) layout {
	if id == 0 {
		root := layout{ID: 0, Props: map[string]dbus.Variant{"children-display": dbus.MakeVariant("submenu")}}
		for i := range m.items {
			root.Children = append(root.Children, dbus.MakeVariant(m.node(int32(i+1))))
		}
		return root
	}
	it := m.items[id-1]
	if it.Label == "" {
		return layout{ID: id, Props: map[string]dbus.Variant{"type": dbus.MakeVariant("separator")}, Children: []dbus.Variant{}}
	}
	return layout{ID: id, Props: map[string]dbus.Variant{"label": dbus.MakeVariant(it.Label), "enabled": dbus.MakeVariant(true), "visible": dbus.MakeVariant(true)}, Children: []dbus.Variant{}}
}

func (m *menu) GetLayout(parent, depth int32, names []string) (uint32, layout, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if parent < 0 || int(parent) > len(m.items) {
		return 0, layout{}, dbus.MakeFailedError(fmt.Errorf("no item %d", parent))
	}
	return m.revision, m.node(parent), nil
}

type groupProps struct {
	ID    int32
	Props map[string]dbus.Variant
}

func (m *menu) GetGroupProperties(ids []int32, names []string) ([]groupProps, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []groupProps
	for _, id := range ids {
		if id >= 0 && int(id) <= len(m.items) {
			out = append(out, groupProps{id, m.node(id).Props})
		}
	}
	return out, nil
}

func (m *menu) GetProperty(id int32, name string) (dbus.Variant, *dbus.Error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id < 0 || int(id) > len(m.items) {
		return dbus.Variant{}, dbus.MakeFailedError(fmt.Errorf("no item %d", id))
	}
	return m.node(id).Props[name], nil
}

// Event is an item used: "clicked" chooses it.
func (m *menu) Event(id int32, event string, data dbus.Variant, at uint32) *dbus.Error {
	m.mu.Lock()
	var chosen func()
	if event == "clicked" && id >= 1 && int(id) <= len(m.items) {
		chosen = m.items[id-1].Chosen
	}
	m.mu.Unlock()
	if chosen != nil {
		go chosen()
	}
	return nil
}

type event struct {
	ID    int32
	Event string
	Data  dbus.Variant
	At    uint32
}

func (m *menu) EventGroup(events []event) ([]int32, *dbus.Error) {
	for _, e := range events {
		m.Event(e.ID, e.Event, e.Data, e.At)
	}
	return []int32{}, nil
}

// AboutToShow says nothing changed: the menu is always the same.
func (m *menu) AboutToShow(id int32) (bool, *dbus.Error) { return false, nil }

func (m *menu) AboutToShowGroup(ids []int32) ([]int32, []int32, *dbus.Error) {
	return []int32{}, []int32{}, nil
}
