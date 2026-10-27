local user_input = os.getenv("USER_INPUT")
local fn = loadstring(user_input)
if fn then fn() end
