-- asset_id is an optional field that can be used to specify the asset id that
-- will be used to pay the swap invoice using a taproot assets channel.
ALTER TABLE loopout_swaps ADD asset_id BYTEA;

-- asset_edge_node is an optional field that can be used to specify the node
-- that will be used to route the asset payment through.
ALTER TABLE loopout_swaps ADD asset_edge_node BYTEA;