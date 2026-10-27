defmodule UnsafeEval do
  def run(user_input) do
    Code.eval_string(user_input)
  end
end
