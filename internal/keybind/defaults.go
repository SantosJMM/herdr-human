package keybind

// defaultKeys son los atajos por defecto de herdr que se pueden enlazar «sin
// querer» con un atajo propio. Salen del config-reference de herdr v0.9.5
// (https://github.com/herdrdev/herdr, docs/next/website/src/data/config-reference.json).
//
// Faltan a propósito los que valen solo en el modo de navegación
// (navigate_*: h, j, k, l, up, down) y los «unset». Es una copia: si herdr cambia
// sus defaults, esta lista se queda atrás, y por eso un choque con ella se avisa
// pero no se bloquea.
var defaultKeys = []struct{ name, key string }{
	{"prefix", "ctrl+b"},
	{"help", "prefix+?"},
	{"settings", "prefix+s"},
	{"new_workspace", "prefix+shift+n"},
	{"new_worktree", "prefix+shift+g"},
	{"rename_workspace", "prefix+shift+w"},
	{"close_workspace", "prefix+shift+d"},
	{"workspace_picker", "prefix+w"},
	{"goto", "prefix+g"},
	{"detach", "prefix+q"},
	{"reload_config", "prefix+shift+r"},
	{"open_notification_target", "prefix+o"},
	{"remote_image_paste", "ctrl+v"},
	{"new_tab", "prefix+c"},
	{"rename_tab", "prefix+shift+t"},
	{"previous_tab", "prefix+p"},
	{"next_tab", "prefix+n"},
	{"switch_tab", "prefix+1..9"},
	{"close_tab", "prefix+shift+x"},
	{"rename_pane", "prefix+shift+p"},
	{"edit_scrollback", "prefix+e"},
	{"copy_mode", "prefix+["},
	{"focus_pane_left", "prefix+h"},
	{"focus_pane_down", "prefix+j"},
	{"focus_pane_up", "prefix+k"},
	{"focus_pane_right", "prefix+l"},
	{"swap_pane_left", "prefix+shift+h"},
	{"swap_pane_down", "prefix+shift+j"},
	{"swap_pane_up", "prefix+shift+k"},
	{"swap_pane_right", "prefix+shift+l"},
	{"cycle_pane_next", "prefix+tab"},
	{"cycle_pane_previous", "prefix+shift+tab"},
	{"split_vertical", "prefix+v"},
	{"split_horizontal", "prefix+minus"},
	{"close_pane", "prefix+x"},
	{"zoom", "prefix+z"},
	{"resize_mode", "prefix+r"},
	{"toggle_sidebar", "prefix+b"},
}
